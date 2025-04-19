// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import (
	types "github.com/hyperledger-labs/SmartBFT/pkg/types"
	mock "github.com/stretchr/testify/mock"
)

// AssemblerMock is an autogenerated mock type for the AssemblerMock type
type AssemblerMock struct {
	mock.Mock
}

// AssembleProposal provides a mock function with given fields: metadata, requests
func (_m *AssemblerMock) AssembleProposal(metadata []byte, requests [][]byte) types.Proposal {
	ret := _m.Called(metadata, requests)

	var r0 types.Proposal
	if rf, ok := ret.Get(0).(func([]byte, [][]byte) types.Proposal); ok {
		r0 = rf(metadata, requests)
	} else {
		r0 = ret.Get(0).(types.Proposal)
	}

	return r0
}
