package types_test

import (
	"time"

	commitmenttypes "github.com/cosmos/ibc-go/v3/modules/core/23-commitment/types"
	"github.com/cosmos/ibc-go/v3/modules/core/exported"
	"github.com/cosmos/ibc-go/v3/modules/light-clients/01-dymint/types"
)

func (suite *DymintTestSuite) TestConsensusStateValidateBasic() {
	testCases := []struct {
		msg            string
		consensusState *types.ConsensusState
		expectPass     bool
	}{
		{
			"success",
			&types.ConsensusState{
				Timestamp:          suite.now,
				Root:               commitmenttypes.NewMerkleRoot([]byte("app_hash")),
				NextValidatorsHash: suite.valsHash,
			},
			true,
		},
		{
			"success with sentinel",
			&types.ConsensusState{
				Timestamp:          suite.now,
				Root:               commitmenttypes.NewMerkleRoot([]byte(types.SentinelRoot)),
				NextValidatorsHash: suite.valsHash,
			},
			true,
		},
		{
			"root is nil",
			&types.ConsensusState{
				Timestamp:          suite.now,
				Root:               commitmenttypes.MerkleRoot{},
				NextValidatorsHash: suite.valsHash,
			},
			false,
		},
		{
			"root is empty",
			&types.ConsensusState{
				Timestamp:          suite.now,
				Root:               commitmenttypes.MerkleRoot{},
				NextValidatorsHash: suite.valsHash,
			},
			false,
		},

		{
			"timestamp is zero",
			&types.ConsensusState{
				Timestamp:          time.Time{},
				Root:               commitmenttypes.NewMerkleRoot([]byte("app_hash")),
				NextValidatorsHash: suite.valsHash,
			},
			false,
		},
	}

	for i, tc := range testCases {
		tc := tc

		// check just to increase coverage
		suite.Require().Equal(exported.Dymint, tc.consensusState.ClientType())
		suite.Require().Equal(tc.consensusState.GetRoot(), tc.consensusState.Root)

		err := tc.consensusState.ValidateBasic()
		if tc.expectPass {
			suite.Require().NoError(err, "valid test case %d failed: %s", i, tc.msg)
		} else {
			suite.Require().Error(err, "invalid test case %d passed: %s", i, tc.msg)
		}
	}
}
