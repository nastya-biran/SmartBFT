package chain

import (
	"encoding/json"
)

type Transaction struct {
	ClientID string
	ID       string
}

type Block struct {
	Sequence     uint64
	PrevHash     string
	Transactions []Transaction
}

type BlockHeader struct {
	Sequence int64
	PrevHash string
	DataHash string
}

type BlockData struct {
	Transactions [][]byte
}

func (t Transaction) ToBytes() []byte {
	bytes, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	return bytes
}

func TransactionFromBytes(bytes []byte) Transaction {
	var t Transaction
	if err := json.Unmarshal(bytes, &t); err != nil {
		panic(err)
	}
	return t
}

func (b BlockHeader) ToBytes() []byte {
	bytes, err := json.Marshal(b)
	if err != nil {
		panic(err)
	}
	return bytes
}

func BlockHeaderFromBytes(bytes []byte) BlockHeader {
	var h BlockHeader
	if err := json.Unmarshal(bytes, &h); err != nil {
		panic(err)
	}
	return h
}

func (b BlockData) ToBytes() []byte {
	bytes, err := json.Marshal(b)
	if err != nil {
		panic(err)
	}
	return bytes
}

func BlockDataFromBytes(bytes []byte) BlockData {
	var d BlockData
	if err := json.Unmarshal(bytes, &d); err != nil {
		panic(err)
	}
	return d
} 