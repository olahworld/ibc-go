package keeper

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	"github.com/tendermint/tendermint/libs/log"

	"github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	host "github.com/cosmos/ibc-go/v3/modules/core/24-host"
	"github.com/cosmos/ibc-go/v3/modules/core/exported"
)

// Keeper represents a type that grants read and write permissions to any client
// state information
type Keeper struct {
	storeKey      sdk.StoreKey
	cdc           codec.BinaryCodec
	paramSpace    paramtypes.Subspace
	stakingKeeper types.StakingKeeper
	upgradeKeeper types.UpgradeKeeper
	selfClient    exported.SelfClient
	clientHooks   exported.ClientHooks
}

// NewKeeper creates a new NewKeeper instance
func NewKeeper(
	cdc codec.BinaryCodec,
	key sdk.StoreKey,
	paramSpace paramtypes.Subspace,
	sk types.StakingKeeper,
	uk types.UpgradeKeeper,
	selfClient exported.SelfClient,
	clientHooks exported.ClientHooks) Keeper {
	// set KeyTable if it has not already been set
	if !paramSpace.HasKeyTable() {
		paramSpace = paramSpace.WithKeyTable(types.ParamKeyTable())
	}

	return Keeper{
		storeKey:      key,
		cdc:           cdc,
		paramSpace:    paramSpace,
		stakingKeeper: sk,
		upgradeKeeper: uk,
		selfClient:    selfClient,
		clientHooks:   clientHooks,
	}
}

// Logger returns a module-specific logger.
func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", "x/"+host.ModuleName+"/"+types.SubModuleName)
}

// GenerateClientIdentifier returns the next client identifier.
func (k Keeper) GenerateClientIdentifier(ctx sdk.Context, clientType string) string {
	nextClientSeq := k.GetNextClientSequence(ctx)
	clientID := types.FormatClientIdentifier(clientType, nextClientSeq)

	nextClientSeq++
	k.SetNextClientSequence(ctx, nextClientSeq)
	return clientID
}

// GetClientState gets a particular client from the store
func (k Keeper) GetClientState(ctx sdk.Context, clientID string) (exported.ClientState, bool) {
	store := k.ClientStore(ctx, clientID)
	bz := store.Get(host.ClientStateKey())
	if bz == nil {
		return nil, false
	}

	clientState := k.MustUnmarshalClientState(bz)
	return clientState, true
}

// SetClientState sets a particular Client to the store
func (k Keeper) SetClientState(ctx sdk.Context, clientID string, clientState exported.ClientState) {
	store := k.ClientStore(ctx, clientID)
	store.Set(host.ClientStateKey(), k.MustMarshalClientState(clientState))
}

// GetClientConsensusState gets the stored consensus state from a client at a given height.
func (k Keeper) GetClientConsensusState(ctx sdk.Context, clientID string, height exported.Height) (exported.ConsensusState, bool) {
	store := k.ClientStore(ctx, clientID)
	bz := store.Get(host.ConsensusStateKey(height))
	if bz == nil {
		return nil, false
	}

	consensusState := k.MustUnmarshalConsensusState(bz)
	return consensusState, true
}

// SetClientConsensusState sets a ConsensusState to a particular client at the given
// height
func (k Keeper) SetClientConsensusState(ctx sdk.Context, clientID string, height exported.Height, consensusState exported.ConsensusState) {
	store := k.ClientStore(ctx, clientID)
	store.Set(host.ConsensusStateKey(height), k.MustMarshalConsensusState(consensusState))
}

// GetNextClientSequence gets the next client sequence from the store.
func (k Keeper) GetNextClientSequence(ctx sdk.Context) uint64 {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get([]byte(types.KeyNextClientSequence))
	if bz == nil {
		panic("next client sequence is nil")
	}

	return sdk.BigEndianToUint64(bz)
}

// SetNextClientSequence sets the next client sequence to the store.
func (k Keeper) SetNextClientSequence(ctx sdk.Context, sequence uint64) {
	store := ctx.KVStore(k.storeKey)
	bz := sdk.Uint64ToBigEndian(sequence)
	store.Set([]byte(types.KeyNextClientSequence), bz)
}

// IterateConsensusStates provides an iterator over all stored consensus states.
// objects. For each State object, cb will be called. If the cb returns true,
// the iterator will close and stop.
func (k Keeper) IterateConsensusStates(ctx sdk.Context, cb func(clientID string, cs types.ConsensusStateWithHeight) bool) {
	store := ctx.KVStore(k.storeKey)
	iterator := sdk.KVStorePrefixIterator(store, host.KeyClientStorePrefix)

	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		keySplit := strings.Split(string(iterator.Key()), "/")
		// consensus key is in the format "clients/<clientID>/consensusStates/<height>"
		if len(keySplit) != 4 || keySplit[2] != string(host.KeyConsensusStatePrefix) {
			continue
		}
		clientID := keySplit[1]
		height := types.MustParseHeight(keySplit[3])
		consensusState := k.MustUnmarshalConsensusState(iterator.Value())

		consensusStateWithHeight := types.NewConsensusStateWithHeight(height, consensusState)

		if cb(clientID, consensusStateWithHeight) {
			break
		}
	}
}

// GetAllGenesisClients returns all the clients in state with their client ids returned as IdentifiedClientState
func (k Keeper) GetAllGenesisClients(ctx sdk.Context) types.IdentifiedClientStates {
	var genClients types.IdentifiedClientStates
	k.IterateClients(ctx, func(clientID string, cs exported.ClientState) bool {
		genClients = append(genClients, types.NewIdentifiedClientState(clientID, cs))
		return false
	})

	return genClients.Sort()
}

// GetAllClientMetadata will take a list of IdentifiedClientState and return a list
// of IdentifiedGenesisMetadata necessary for exporting and importing client metadata
// into the client store.
func (k Keeper) GetAllClientMetadata(ctx sdk.Context, genClients []types.IdentifiedClientState) ([]types.IdentifiedGenesisMetadata, error) {
	genMetadata := make([]types.IdentifiedGenesisMetadata, 0)
	for _, ic := range genClients {
		cs, err := types.UnpackClientState(ic.ClientState)
		if err != nil {
			return nil, err
		}
		gms := cs.ExportMetadata(k.ClientStore(ctx, ic.ClientId))
		if len(gms) == 0 {
			continue
		}
		clientMetadata := make([]types.GenesisMetadata, len(gms))
		for i, metadata := range gms {
			cmd, ok := metadata.(types.GenesisMetadata)
			if !ok {
				return nil, sdkerrors.Wrapf(types.ErrInvalidClientMetadata, "expected metadata type: %T, got: %T",
					types.GenesisMetadata{}, cmd)
			}
			clientMetadata[i] = cmd
		}
		genMetadata = append(genMetadata, types.NewIdentifiedGenesisMetadata(
			ic.ClientId,
			clientMetadata,
		))
	}
	return genMetadata, nil
}

// SetAllClientMetadata takes a list of IdentifiedGenesisMetadata and stores all of the metadata in the client store at the appropriate paths.
func (k Keeper) SetAllClientMetadata(ctx sdk.Context, genMetadata []types.IdentifiedGenesisMetadata) {
	for _, igm := range genMetadata {
		// create client store
		store := k.ClientStore(ctx, igm.ClientId)
		// set all metadata kv pairs in client store
		for _, md := range igm.ClientMetadata {
			store.Set(md.GetKey(), md.GetValue())
		}
	}
}

// GetAllConsensusStates returns all stored client consensus states.
func (k Keeper) GetAllConsensusStates(ctx sdk.Context) types.ClientsConsensusStates {
	clientConsStates := make(types.ClientsConsensusStates, 0)
	mapClientIDToConsStateIdx := make(map[string]int)

	k.IterateConsensusStates(ctx, func(clientID string, cs types.ConsensusStateWithHeight) bool {
		idx, ok := mapClientIDToConsStateIdx[clientID]
		if ok {
			clientConsStates[idx].ConsensusStates = append(clientConsStates[idx].ConsensusStates, cs)
			return false
		}

		clientConsState := types.ClientConsensusStates{
			ClientId:        clientID,
			ConsensusStates: []types.ConsensusStateWithHeight{cs},
		}

		clientConsStates = append(clientConsStates, clientConsState)
		mapClientIDToConsStateIdx[clientID] = len(clientConsStates) - 1
		return false
	})

	return clientConsStates.Sort()
}

// HasClientConsensusState returns if keeper has a ConsensusState for a particular
// client at the given height
func (k Keeper) HasClientConsensusState(ctx sdk.Context, clientID string, height exported.Height) bool {
	store := k.ClientStore(ctx, clientID)
	return store.Has(host.ConsensusStateKey(height))
}

// GetLatestClientConsensusState gets the latest ConsensusState stored for a given client
func (k Keeper) GetLatestClientConsensusState(ctx sdk.Context, clientID string) (exported.ConsensusState, bool) {
	clientState, ok := k.GetClientState(ctx, clientID)
	if !ok {
		return nil, false
	}
	return k.GetClientConsensusState(ctx, clientID, clientState.GetLatestHeight())
}

// GetSelfConsensusState introspects the (self) past historical info at a given height
// and returns the expected consensus state at that height.
// For now, can only retrieve self consensus states for the current revision
func (k Keeper) GetSelfConsensusState(ctx sdk.Context, height exported.Height) (exported.ConsensusState, error) {
	selfHeight, ok := height.(types.Height)
	if !ok {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrInvalidType, "expected %T, got %T", types.Height{}, height)
	}
	// check that height revision matches chainID revision
	revision := types.ParseChainID(ctx.ChainID())
	if revision != height.GetRevisionNumber() {
		return nil, sdkerrors.Wrapf(types.ErrInvalidHeight, "chainID revision number does not match height revision number: expected %d, got %d", revision, height.GetRevisionNumber())
	}
	histInfo, found := k.stakingKeeper.GetHistoricalInfo(ctx, int64(selfHeight.RevisionHeight))
	if !found {
		return nil, sdkerrors.Wrapf(sdkerrors.ErrNotFound, "no historical info found at height %d", selfHeight.RevisionHeight)
	}

	blockHeader, err := histInfo.Header.Marshal()
	if err != nil {
		return nil, err
	}

	return k.selfClient.GetSelfConsensusStateFromBlocHeader(k.cdc, blockHeader)
}

// ValidateSelfClient validates the client parameters for a client of the running chain
// This function is only used to validate the client state the counterparty stores for this chain
// Client must be in same revision as the executing chain
func (k Keeper) ValidateSelfClient(ctx sdk.Context, clientState exported.ClientState) error {
	if k.selfClient.ClientType() != clientState.ClientType() {
		return sdkerrors.Wrapf(types.ErrInvalidClient, "invalid client type. expected: %s, got: %s",
			k.selfClient.ClientType(), clientState.ClientType())
	}
	return k.selfClient.ValidateSelfClientState(ctx, k.stakingKeeper.UnbondingTime(ctx), clientState)
}

// GetUpgradePlan executes the upgrade keeper GetUpgradePlan function.
func (k Keeper) GetUpgradePlan(ctx sdk.Context) (plan upgradetypes.Plan, havePlan bool) {
	return k.upgradeKeeper.GetUpgradePlan(ctx)
}

// GetUpgradedClient executes the upgrade keeper GetUpgradeClient function.
func (k Keeper) GetUpgradedClient(ctx sdk.Context, planHeight int64) ([]byte, bool) {
	return k.upgradeKeeper.GetUpgradedClient(ctx, planHeight)
}

// GetUpgradedConsensusState returns the upgraded consensus state
func (k Keeper) GetUpgradedConsensusState(ctx sdk.Context, planHeight int64) ([]byte, bool) {
	return k.upgradeKeeper.GetUpgradedConsensusState(ctx, planHeight)
}

// SetUpgradedConsensusState executes the upgrade keeper SetUpgradedConsensusState function.
func (k Keeper) SetUpgradedConsensusState(ctx sdk.Context, planHeight int64, bz []byte) error {
	return k.upgradeKeeper.SetUpgradedConsensusState(ctx, planHeight, bz)
}

// IterateClients provides an iterator over all stored light client State
// objects. For each State object, cb will be called. If the cb returns true,
// the iterator will close and stop.
func (k Keeper) IterateClients(ctx sdk.Context, cb func(clientID string, cs exported.ClientState) bool) {
	store := ctx.KVStore(k.storeKey)
	iterator := sdk.KVStorePrefixIterator(store, host.KeyClientStorePrefix)

	defer iterator.Close()
	for ; iterator.Valid(); iterator.Next() {
		keySplit := strings.Split(string(iterator.Key()), "/")
		if keySplit[len(keySplit)-1] != host.KeyClientState {
			continue
		}
		clientState := k.MustUnmarshalClientState(iterator.Value())

		// key is ibc/{clientid}/clientState
		// Thus, keySplit[1] is clientID
		if cb(keySplit[1], clientState) {
			break
		}
	}
}

// GetAllClients returns all stored light client State objects.
func (k Keeper) GetAllClients(ctx sdk.Context) (states []exported.ClientState) {
	k.IterateClients(ctx, func(_ string, state exported.ClientState) bool {
		states = append(states, state)
		return false
	})
	return states
}

// ClientStore returns isolated prefix store for each client so they can read/write in separate
// namespace without being able to read/write other client's data
func (k Keeper) ClientStore(ctx sdk.Context, clientID string) sdk.KVStore {
	clientPrefix := []byte(fmt.Sprintf("%s/%s/", host.KeyClientStorePrefix, clientID))
	return prefix.NewStore(ctx.KVStore(k.storeKey), clientPrefix)
}
