package types

import (
	"time"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	tmbytes "github.com/tendermint/tendermint/libs/bytes"

	clienttypes "github.com/cosmos/ibc-go/v3/modules/core/02-client/types"
	commitmenttypes "github.com/cosmos/ibc-go/v3/modules/core/23-commitment/types"
	"github.com/cosmos/ibc-go/v3/modules/core/exported"
)

// SentinelRoot is used as a stand-in root value for the consensus state set at the upgrade height
const SentinelRoot = "sentinel_root"

// NewConsensusState creates a new ConsensusState instance.
func NewConsensusState(
	timestamp time.Time, root commitmenttypes.MerkleRoot, nextValsHash tmbytes.HexBytes,
) *ConsensusState {
	return &ConsensusState{
		Timestamp: timestamp,
		Root:      root,
	}
}

// ClientType returns Dymint
func (ConsensusState) ClientType() string {
	return exported.Dymint
}

// GetRoot returns the commitment Root for the specific
func (cs ConsensusState) GetRoot() exported.Root {
	return cs.Root
}

// GetTimestamp returns block time in nanoseconds of the header that created consensus state
func (cs ConsensusState) GetTimestamp() uint64 {
	return uint64(cs.Timestamp.UnixNano())
}

// ValidateBasic defines a basic validation for the dymint consensus state.
// NOTE: ProcessedTimestamp may be zero if this is an initial consensus state passed in by relayer
// as opposed to a consensus state constructed by the chain.
func (cs ConsensusState) ValidateBasic() error {
	if cs.Root.Empty() {
		return sdkerrors.Wrap(clienttypes.ErrInvalidConsensus, "root cannot be empty")
	}
	if cs.Timestamp.Unix() <= 0 {
		return sdkerrors.Wrap(clienttypes.ErrInvalidConsensus, "timestamp must be a positive Unix time")
	}
	return nil
}
