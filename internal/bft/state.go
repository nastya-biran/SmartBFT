// Copyright IBM Corp. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bft

import (
	"fmt"

	"github.com/SmartBFT-Go/consensus/pkg/api"
	"github.com/SmartBFT-Go/consensus/pkg/types"
	"github.com/SmartBFT-Go/consensus/smartbftprotos"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type StateRecorder struct {
	SavedMessages []*smartbftprotos.SavedMessage
}

func (sr *StateRecorder) Save(message *smartbftprotos.SavedMessage) error {
	sr.SavedMessages = append(sr.SavedMessages, message)
	return nil
}

func (*StateRecorder) Restore(_ *View) error {
	panic("should not be used")
}

type PersistedState struct {
	InFlightProposal *InFlightData
	Entries          [][]byte
	Logger           api.Logger
	WAL              api.WriteAheadLog
}

func (ps *PersistedState) Save(msgToSave *smartbftprotos.SavedMessage) error {
	if proposed := msgToSave.GetProposedRecord(); proposed != nil {
		ps.storeProposal(proposed)
	}
	if prepared := msgToSave.GetPreparedProof(); prepared != nil {
		ps.storePrepared(prepared)
	}

	b, err := proto.Marshal(msgToSave)
	if err != nil {
		ps.Logger.Panicf("Failed marshaling message: %v", err)
	}
	// It is only safe to truncate if we either:
	//
	// 1) Process a pre-prepare, because it means we safely persisted the
	// previous proposal and have a stable checkpoint.
	// 2) Acquired a new view message which contains 2f+1 attestations
	//    of the cluster agreeing to a new view configuration.
	newProposal := msgToSave.GetProposedRecord() != nil
	// TODO: handle view message here as well, and add "|| finalizedView" to truncate flag
	return ps.WAL.Append(b, newProposal)
}

func (ps *PersistedState) storeProposal(proposed *smartbftprotos.ProposedRecord) {
	proposal := proposed.PrePrepare.Proposal
	proposalToStore := types.Proposal{
		VerificationSequence: int64(proposal.VerificationSequence),
		Header:               proposal.Header,
		Payload:              proposal.Payload,
		Metadata:             proposal.Metadata,
	}
	ps.InFlightProposal.StoreProposal(proposalToStore)
}

func (ps *PersistedState) storePrepared(prepared *smartbftprotos.PreparedProof) {
	cmt := prepared.Commit.GetCommit()
	ps.InFlightProposal.StorePrepares(cmt.View, cmt.Seq)
}

func (ps *PersistedState) Restore(v *View) error {
	// Unless we conclude otherwise, we're in a COMMITTED state
	v.Phase = COMMITTED

	entries := ps.Entries
	if len(entries) == 0 {
		ps.Logger.Infof("Nothing to restore")
		return nil
	}

	ps.Logger.Infof("WAL contains %d entries", len(entries))

	lastEntry := entries[len(entries)-1]
	lastPersistedMessage := &smartbftprotos.SavedMessage{}
	if err := proto.Unmarshal(lastEntry, lastPersistedMessage); err != nil {
		ps.Logger.Errorf("Failed unmarshaling last entry from WAL: %v", err)
		return errors.Wrap(err, "failed unmarshaling last entry from WAL")
	}

	if proposed := lastPersistedMessage.GetProposedRecord(); proposed != nil {
		return ps.recoverProposed(proposed, v)
	}

	if preparedProof := lastPersistedMessage.GetPreparedProof(); preparedProof != nil {
		return ps.recoverPrepared(preparedProof, v, entries)
	}

	// TODO: handle signed view data persisted in the WAL
	return errors.Errorf("unrecognized record: %v", lastPersistedMessage)
}

func (ps *PersistedState) recoverProposed(lastPersistedMessage *smartbftprotos.ProposedRecord, v *View) error {
	prop := lastPersistedMessage.GetPrePrepare().Proposal
	v.inFlightProposal = &types.Proposal{
		VerificationSequence: int64(prop.VerificationSequence),
		Metadata:             prop.Metadata,
		Payload:              prop.Payload,
		Header:               prop.Header,
	}
	ps.storeProposal(lastPersistedMessage)
	// Reconstruct the prepare message we shall next broadcast
	// after the recovery.
	prp := lastPersistedMessage.GetPrePrepare()
	v.lastBroadcastSent = &smartbftprotos.Message{
		Content: &smartbftprotos.Message_Prepare{
			Prepare: lastPersistedMessage.GetPrepare(),
		},
	}
	v.Phase = PROPOSED
	v.Number = prp.View
	v.ProposalSequence = prp.Seq
	ps.Logger.Infof("Restored proposal with sequence %d", lastPersistedMessage.GetPrePrepare().Seq)
	return nil
}

func (ps *PersistedState) recoverPrepared(lastPersistedMessage *smartbftprotos.PreparedProof, v *View, entries [][]byte) error {
	// Last entry is a commit, so we should have not pruned the previous pre-prepare
	if len(entries) < 2 {
		return fmt.Errorf("last message is a commit, but expected to also have a matching pre-prepare")
	}
	prePrepareMsg := &smartbftprotos.SavedMessage{}
	if err := proto.Unmarshal(entries[len(entries)-2], prePrepareMsg); err != nil {
		ps.Logger.Errorf("Failed unmarshaling second last entry from WAL: %v", err)
		return errors.Wrap(err, "failed unmarshaling last entry from WAL")
	}

	prePrepareFromWAL := prePrepareMsg.GetProposedRecord().GetPrePrepare()

	if prePrepareFromWAL == nil {
		return fmt.Errorf("expected second last message to be a pre-prepare, but got '%v' instead", prePrepareMsg)
	}

	if v.ProposalSequence < prePrepareFromWAL.Seq {
		err := fmt.Errorf("last proposal sequence persisted into WAL is %d which is greater than last committed sequence is %d", prePrepareFromWAL.Seq, v.ProposalSequence)
		ps.Logger.Errorf("Failed recovery: %s", err)
		return err
	}

	// Check if the WAL's last sequence has been persisted into the application layer.
	if v.ProposalSequence > prePrepareFromWAL.Seq {
		ps.Logger.Infof("Last proposal with sequence %d has been safely committed", v.ProposalSequence)
		return nil
	}

	// Else, v.ProposalSequence == prePrepareFromWAL.Seq

	prop := prePrepareFromWAL.Proposal
	v.inFlightProposal = &types.Proposal{
		VerificationSequence: int64(prop.VerificationSequence),
		Metadata:             prop.Metadata,
		Payload:              prop.Payload,
		Header:               prop.Header,
	}
	ps.storeProposal(prePrepareMsg.GetProposedRecord())
	ps.storePrepared(lastPersistedMessage)

	// Reconstruct the commit message we shall next broadcast
	// after the recovery.
	v.lastBroadcastSent = lastPersistedMessage.GetCommit()
	v.Phase = PREPARED
	v.Number = prePrepareFromWAL.View
	v.ProposalSequence = prePrepareFromWAL.Seq

	// Restore signature
	signatureInLastSentCommit := v.lastBroadcastSent.GetCommit().Signature
	v.myProposalSig = &types.Signature{
		Id:    signatureInLastSentCommit.Signer,
		Msg:   signatureInLastSentCommit.Msg,
		Value: signatureInLastSentCommit.Value,
	}

	ps.Logger.Infof("Restored proposal with sequence %d", prePrepareFromWAL.Seq)
	return nil
}
