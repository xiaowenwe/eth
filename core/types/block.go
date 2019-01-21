// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package types contains data types related to Ethereum consensus.
package types

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	EmptyRootHash  = DeriveSha(Transactions{})
	EmptyUncleHash = CalcUncleHash(nil)
)

// A BlockNonce is a 64-bit hash which proves (combined with the
// mix-hash) that a sufficient amount of computation has been carried
// out on a block.
//BlockNone是一个64位哈希，它证明(与//Mix-散列)已进行了足够数量的计算//在一个区块外。
type BlockNonce [8]byte

//将给定的整数转换为块Nonce。
// EncodeNonce converts the given integer to a block nonce.
func EncodeNonce(i uint64) BlockNonce {
	var n BlockNonce
	binary.BigEndian.PutUint64(n[:], i)
	return n
}

// Uint64 returns the integer value of a block nonce.
//返回块的整数值
func (n BlockNonce) Uint64() uint64 {
	return binary.BigEndian.Uint64(n[:])
}

// MarshalText encodes n as a hex string with 0x prefix.
//将n编码为带有0x前缀的十六进制字符串
func (n BlockNonce) MarshalText() ([]byte, error) {
	return hexutil.Bytes(n[:]).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
//实现encoding.TextUnmarshaler 将输入解码为带有0x前缀的字符串。外的长度确定所需输入长度。此函数通常用于实现固定尺寸类型的UnmarshalText方法。
func (n *BlockNonce) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("BlockNonce", input, n[:])
}

//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a block header in the Ethereum blockchain.//区块头
type Header struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"` //该区块的父区块的Hash值
	UncleHash   common.Hash    `json:"sha3Uncles"       gencodec:"required"` //该区块的叔区块的Hash值
	Coinbase    common.Address `json:"miner"            gencodec:"required"` //打包该区块矿工的地址，矿工费和发现区块的奖励会被发送到该地址
	Root        common.Hash    `json:"stateRoot"        gencodec:"required"` //Merkle树根节点的Hash，以太坊中的交易状态信息是以Merkle状态树的形式进行存储的，Root是该状态树的根节点的Hash值
	TxHash      common.Hash    `json:"transactionsRoot" gencodec:"required"` //保存该区块中交易Merkle树的根节点的Hash值
	ReceiptHash common.Hash    `json:"receiptsRoot"     gencodec:"required"` //收据的哈希/一个区块中所包含的交易中的接收者也是以Merkle树的形式进行存储的，该值是该Merkle树根节点的Hash值
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"` //用于索引与搜索的结构
	Difficulty  *big.Int       `json:"difficulty"       gencodec:"required"` //该区块的难度
	Number      *big.Int       `json:"number"           gencodec:"required"` //所有祖先区块的数量（也就是区块高度）
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"` //该区块的gas上限
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"` //该区块使用的gas
	Time        *big.Int       `json:"timestamp"        gencodec:"required"` //区块开始打包的时间
	Extra       []byte         `json:"extraData"        gencodec:"required"` //区块相关的附加信息
	MixDigest   common.Hash    `json:"mixHash"          gencodec:"required"` //该哈希值与Nonce值一起能够证明在该区块上已经进行了足够的计算（用于验证该区块挖矿成功与否的Hash值
	Nonce       BlockNonce     `json:"nonce"            gencodec:"required"` //一个64bit的哈希数，它被应用在区块的"挖掘"阶段，并且在使用中会被修改。该哈希值与MixDigest值一起能够证明在该区块上已经进行了足够的计算（用于验证该区块挖矿成功与否的Hash值
}

// field type overrides for gencodec字段类型覆盖用于gencodec
type headerMarshaling struct {
	Difficulty *hexutil.Big   //该区块的难度
	Number     *hexutil.Big   //所有祖先区块的数量（也就是区块高度）
	GasLimit   hexutil.Uint64 //该区块的gas上限
	GasUsed    hexutil.Uint64 //该区块使用的gas
	Time       *hexutil.Big   //区块开始打包的时间
	Extra      hexutil.Bytes  //区块相关的附加信息
	Hash       common.Hash    `json:"hash"` // adds call to Hash() in MarshalJSON在MarshalJSON中添加对Hash()的调用
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
//hash返回报头的块哈希，这只是它的KECAK256哈希。
//RLP编码
func (h *Header) Hash() common.Hash {
	return rlpHash(h)
}

// HashNoNonce returns the hash which is used as input for the proof-of-work search.返回用作工作证明搜索的输入的散列
func (h *Header) HashNoNonce() common.Hash {
	return rlpHash([]interface{}{
		h.ParentHash,
		h.UncleHash,
		h.Coinbase,
		h.Root,
		h.TxHash,
		h.ReceiptHash,
		h.Bloom,
		h.Difficulty,
		h.Number,
		h.GasLimit,
		h.GasUsed,
		h.Time,
		h.Extra,
	})
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
//size返回所有内部内容使用的近似内存。它是用的以近似和限制各种缓存的内存消耗。
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h)) + common.StorageSize(len(h.Extra)+(h.Difficulty.BitLen()+h.Number.BitLen()+h.Time.BitLen())/8)
}

//rlp编码
func rlpHash(x interface{}) (h common.Hash) {
	hw := sha3.NewKeccak256()
	rlp.Encode(hw, x)
	hw.Sum(h[:0])
	return h
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a block's data contents (transactions and uncles) together.
//body是一个简单的(可变的、不安全的)数据容器，用于存储和移动区块的数据内容(交易列表和叔块)在一起
type Body struct {
	Transactions []*Transaction
	Uncles       []*Header
}

// Block represents an entire block in the Ethereum blockchain.
//完整的区块
type Block struct {
	header       *Header      //存储的是该区块的信息（结构体为Header）
	uncles       []*Header    //存储的是该区块所包含的叔块（uncle block）的信息
	transactions Transactions //交易列表

	// caches
	hash atomic.Value //并发安全的哈希缓存
	size atomic.Value

	// Td is used by package core to store the total difficulty
	// of the chain up to and including the block.
	//td被包装核心用来存储总难度一直到并包括这个区块
	td *big.Int

	// These fields are used by package eth to track
	// inter-peer block relay.
	//ETH包使用这些字段来跟踪对等块中继
	ReceivedAt   time.Time
	ReceivedFrom interface{}
}

// DeprecatedTd is an old relic for extracting the TD of a block. It is in the
// code solely to facilitate upgrading the database from the old format to the
// new, after which it should be deleted. Do not use!
//DeprecatedTd是提取块TD的旧难度。它在代码仅用于将数据库从旧格式升级到新的，之后应该删除。不要用！
func (b *Block) DeprecatedTd() *big.Int {
	return b.td
}

// [deprecated by eth/63]
// StorageBlock defines the RLP encoding of a Block stored in the
// state database. The StorageBlock encoding contains fields that
// would otherwise need to be recomputed.
//StorageBlock定义存储在块中的块的RLP编码状态数据库。 StorageBlock编码包含的字段否则需要重新计算。
type StorageBlock Block

// "external" block encoding. used for eth protocol, etc.“外部”块编码。 用于eth协议等。
type extblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
}

// [deprecated by eth/63]
// "storage" block encoding. used for database.“存储”块编码。 用于数据库。
type storageblock struct {
	Header *Header
	Txs    []*Transaction
	Uncles []*Header
	TD     *big.Int
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//NewBlock创建一个新块。 输入的数据被复制，对标题和字段值的更改不会影响块。
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
//头部中的TxHash，UncleHash，ReceiptHash和Bloom的值被忽略并被设置为从给定txs，叔叔和收据派生的值。
func NewBlock(header *Header, txs []*Transaction, uncles []*Header, receipts []*Receipt) *Block {
	b := &Block{header: CopyHeader(header), td: new(big.Int)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxHash = EmptyRootHash
	} else {
		b.header.TxHash = DeriveSha(Transactions(txs))
		b.transactions = make(Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptHash = EmptyRootHash
	} else {
		b.header.ReceiptHash = DeriveSha(Receipts(receipts))
		b.header.Bloom = CreateBloom(receipts)
	}

	if len(uncles) == 0 {
		b.header.UncleHash = EmptyUncleHash
	} else {
		b.header.UncleHash = CalcUncleHash(uncles)
		b.uncles = make([]*Header, len(uncles))
		for i := range uncles {
			b.uncles[i] = CopyHeader(uncles[i])
		}
	}

	return b
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
// NewBlockWithHeader用给定的标题数据创建一个块。 标题数据被复制，更改为标题和字段值
//不会影响该块。
func NewBlockWithHeader(header *Header) *Block {
	return &Block{header: CopyHeader(header)}
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
//CopyHeader创建块头的深层副本，以防止副作用修改头变量。
func CopyHeader(h *Header) *Header {
	cpy := *h
	if cpy.Time = new(big.Int); h.Time != nil {
		cpy.Time.Set(h.Time)
	}
	if cpy.Difficulty = new(big.Int); h.Difficulty != nil {
		cpy.Difficulty.Set(h.Difficulty)
	}
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

// DecodeRLP decodes the Ethereum
//RLP解码以太坊
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.uncles, b.transactions = eb.Header, eb.Uncles, eb.Txs
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
//将b序列化为以太坊RLP块格式
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
		Uncles: b.uncles,
	})
}

// [deprecated by eth/63]废弃
func (b *StorageBlock) DecodeRLP(s *rlp.Stream) error {
	var sb storageblock
	if err := s.Decode(&sb); err != nil {
		return err
	}
	b.header, b.uncles, b.transactions, b.td = sb.Header, sb.Uncles, sb.Txs, sb.TD
	return nil
}

// TODO: copies

func (b *Block) Uncles() []*Header          { return b.uncles }
func (b *Block) Transactions() Transactions { return b.transactions }

func (b *Block) Transaction(hash common.Hash) *Transaction {
	for _, transaction := range b.transactions {
		if transaction.Hash() == hash {
			return transaction
		}
	}
	return nil
}

func (b *Block) Number() *big.Int     { return new(big.Int).Set(b.header.Number) }
func (b *Block) GasLimit() uint64     { return b.header.GasLimit }
func (b *Block) GasUsed() uint64      { return b.header.GasUsed }
func (b *Block) Difficulty() *big.Int { return new(big.Int).Set(b.header.Difficulty) }
func (b *Block) Time() *big.Int       { return new(big.Int).Set(b.header.Time) }

func (b *Block) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *Block) MixDigest() common.Hash   { return b.header.MixDigest }
func (b *Block) Nonce() uint64            { return binary.BigEndian.Uint64(b.header.Nonce[:]) }
func (b *Block) Bloom() Bloom             { return b.header.Bloom }
func (b *Block) Coinbase() common.Address { return b.header.Coinbase }
func (b *Block) Root() common.Hash        { return b.header.Root }
func (b *Block) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *Block) TxHash() common.Hash      { return b.header.TxHash }
func (b *Block) ReceiptHash() common.Hash { return b.header.ReceiptHash }
func (b *Block) UncleHash() common.Hash   { return b.header.UncleHash }
func (b *Block) Extra() []byte            { return common.CopyBytes(b.header.Extra) }

func (b *Block) Header() *Header { return CopyHeader(b.header) }

// Body returns the non-header content of the block.
//Body返回该块的非标题内容。
func (b *Block) Body() *Body { return &Body{b.transactions, b.uncles} }

func (b *Block) HashNoNonce() common.Hash {
	return b.header.HashNoNonce()
}

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previsouly cached value.
//Size通过编码和返回块返回块的真正RLP编码存储大小，或返回预先缓存的值。
func (b *Block) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(common.StorageSize(c))
	return common.StorageSize(c)
}

type writeCounter common.StorageSize

func (c *writeCounter) Write(b []byte) (int, error) {
	*c += writeCounter(len(b))
	return len(b), nil
}

//计算叔块哈希
func CalcUncleHash(uncles []*Header) common.Hash {
	return rlpHash(uncles)
}

// WithSeal returns a new block with the data from b but the header replaced with
// the sealed one.
// WithSeal返回一个新的块与来自b的数据，但头被替换为密封的。
func (b *Block) WithSeal(header *Header) *Block {
	cpy := *header

	return &Block{
		header:       &cpy,
		transactions: b.transactions,
		uncles:       b.uncles,
	}
}

// WithBody returns a new block with the given transaction and uncle contents.
//WithBody返回给定交易和叔叔内容的新块。
func (b *Block) WithBody(transactions []*Transaction, uncles []*Header) *Block {
	block := &Block{
		header:       CopyHeader(b.header),
		transactions: make([]*Transaction, len(transactions)),
		uncles:       make([]*Header, len(uncles)),
	}
	copy(block.transactions, transactions)
	for i := range uncles {
		block.uncles[i] = CopyHeader(uncles[i])
	}
	return block
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
// Hash返回b头部的keccak256哈希。
//散列是在第一次调用时计算并随后进行缓存。
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

func (b *Block) String() string {
	str := fmt.Sprintf(`Block(#%v): Size: %v {
MinerHash: %x
%v
Transactions:
%v
Uncles:
%v
}
`, b.Number(), b.Size(), b.header.HashNoNonce(), b.header, b.transactions, b.uncles)
	return str
}

func (h *Header) String() string {
	return fmt.Sprintf(`Header(%x):
[
	ParentHash:	    %x
	UncleHash:	    %x
	Coinbase:	    %x
	Root:		    %x
	TxSha		    %x
	ReceiptSha:	    %x
	Bloom:		    %x
	Difficulty:	    %v
	Number:		    %v
	GasLimit:	    %v
	GasUsed:	    %v
	Time:		    %v
	Extra:		    %s
	MixDigest:      %x
	Nonce:		    %x
]`, h.Hash(), h.ParentHash, h.UncleHash, h.Coinbase, h.Root, h.TxHash, h.ReceiptHash, h.Bloom, h.Difficulty, h.Number, h.GasLimit, h.GasUsed, h.Time, h.Extra, h.MixDigest, h.Nonce)
}

type Blocks []*Block

type BlockBy func(b1, b2 *Block) bool

func (self BlockBy) Sort(blocks Blocks) {
	bs := blockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Sort(bs)
}

type blockSorter struct {
	blocks Blocks
	by     func(b1, b2 *Block) bool
}

func (self blockSorter) Len() int { return len(self.blocks) }
func (self blockSorter) Swap(i, j int) {
	self.blocks[i], self.blocks[j] = self.blocks[j], self.blocks[i]
}
func (self blockSorter) Less(i, j int) bool { return self.by(self.blocks[i], self.blocks[j]) }

func Number(b1, b2 *Block) bool { return b1.header.Number.Cmp(b2.header.Number) < 0 }
