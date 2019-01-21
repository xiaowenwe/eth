// Copyright 2015 The go-ethereum Authors
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

// Contains the node database, storing previously seen nodes and any collected
// metadata about them for QoS purposes.

package discover

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"os"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
	"github.com/syndtr/goleveldb/leveldb/util"
)

//这个文件内部主要实现了节点的持久化，因为p2p网络节点的节点发现和维护都是比较花时间的，为了反复启动的时候，能够把之前的工作继承下来，避免每次都重新发现。 所以持久化的工作是必须的。
//ethdb的代码和trie的代码，trie的持久化工作使用了leveldb。 这里同样也使用了leveldb。 不过p2p的leveldb实例和主要的区块链的leveldb实例不是同一个。
//newNodeDB,根据参数path来看打开基于文件的数据库，还是基于文件的数据库。
var (
	nodeDBNilNodeID      = NodeID{}       // Special node ID to use as a nil element.作为nil元素使用的特殊节点ID。
	nodeDBNodeExpiration = 24 * time.Hour // Time after which an unseen node should be dropped.时间之后，应该删除一个看不见的节点
	nodeDBCleanupCycle   = time.Hour      // Time period for running the expiration task.

)

// nodeDB stores all nodes we know about.
//nodeDB存储我们所了解的所有节点。
type nodeDB struct {
	lvl    *leveldb.DB   // Interface to the database itself数据库本身的接口
	self   NodeID        // Own node id to prevent adding it into the database拥有节点ID以防止将其添加到数据库中
	runner sync.Once     // Ensures we can start at most one expirer确保我们最多可以启动一个到期者
	quit   chan struct{} // Channel to signal the expiring thread to stop通道用信号通知即将到期的线程停止
}

// Schema layout for the node database节点数据库的架构布局
var (
	nodeDBVersionKey = []byte("version") // Version of the database to flush if changes如果发生更改，则刷新数据库的版本
	nodeDBItemPrefix = []byte("n:")      // Identifier to prefix node entries with用于为节点条目添加前缀的标识符

	nodeDBDiscoverRoot      = ":discover"
	nodeDBDiscoverPing      = nodeDBDiscoverRoot + ":lastping"
	nodeDBDiscoverPong      = nodeDBDiscoverRoot + ":lastpong"
	nodeDBDiscoverFindFails = nodeDBDiscoverRoot + ":findfail"
)

//根据参数path来看打开基于文件的数据库，还是基于内存的数据库。
// newNodeDB creates a new node database for storing and retrieving infos about
// known peers in the network. If no path is given, an in-memory, temporary
// database is constructed.
// newNodeDB创建一个新的节点数据库来存储和检索有关的信息
//网络中的已知对等点。 如果没有给出路径，则表示内存中的临时路径
//数据库被构建。
func newNodeDB(path string, version int, self NodeID) (*nodeDB, error) {
	if path == "" {
		return newMemoryNodeDB(self)
	}
	return newPersistentNodeDB(path, version, self)
}

// newMemoryNodeDB creates a new in-memory node database without a persistent
// backend.
// newMemoryNodeDB创建一个新的内存节点数据库，而不需要持久的后端。
func newMemoryNodeDB(self NodeID) (*nodeDB, error) {
	db, err := leveldb.Open(storage.NewMemStorage(), nil)
	if err != nil {
		return nil, err
	}
	return &nodeDB{
		lvl:  db,
		self: self,
		quit: make(chan struct{}),
	}, nil
}

// newPersistentNodeDB creates/opens a leveldb backed persistent node database,
// also flushing its contents in case of a version mismatch.
// newPersistentNodeDB创建/打开一个leveldb支持的永久节点数据库，
//在版本不匹配的情况下也会刷新其内容。
func newPersistentNodeDB(path string, version int, self NodeID) (*nodeDB, error) {
	opts := &opt.Options{OpenFilesCacheCapacity: 5}
	db, err := leveldb.OpenFile(path, opts)
	if _, iscorrupted := err.(*errors.ErrCorrupted); iscorrupted {
		db, err = leveldb.RecoverFile(path, nil)
	}
	if err != nil {
		return nil, err
	}
	// The nodes contained in the cache correspond to a certain protocol version.
	// Flush all nodes if the version doesn't match.
	//缓存中包含的节点对应于某个协议版本。
	//如果版本不匹配，刷新所有节点。
	currentVer := make([]byte, binary.MaxVarintLen64)
	currentVer = currentVer[:binary.PutVarint(currentVer, int64(version))]

	blob, err := db.Get(nodeDBVersionKey, nil)
	switch err {
	case leveldb.ErrNotFound:
		// Version not found (i.e. empty cache), insert it
		if err := db.Put(nodeDBVersionKey, currentVer, nil); err != nil {
			db.Close()
			return nil, err
		}

	case nil:
		// Version present, flush if different
		//版本不同，先删除所有的数据库文件，重新创建一个。
		if !bytes.Equal(blob, currentVer) {
			db.Close()
			if err = os.RemoveAll(path); err != nil {
				return nil, err
			}
			return newPersistentNodeDB(path, version, self)
		}
	}
	return &nodeDB{
		lvl:  db,
		self: self,
		quit: make(chan struct{}),
	}, nil
}

// makeKey generates the leveldb key-blob from a node id and its particular
// field of interest.
// makeKey从节点ID和它感兴趣的特定字段生成leveldb key-blob
func makeKey(id NodeID, field string) []byte {
	if bytes.Equal(id[:], nodeDBNilNodeID[:]) {
		return []byte(field)
	}
	return append(nodeDBItemPrefix, append(id[:], field...)...)
}

// splitKey tries to split a database key into a node id and a field part.
// splitKey尝试将数据库键分割成节点ID和字段部分。
func splitKey(key []byte) (id NodeID, field string) {
	// If the key is not of a node, return it plainly
	if !bytes.HasPrefix(key, nodeDBItemPrefix) {
		return NodeID{}, string(key)
	}
	// Otherwise split the id and field
	item := key[len(nodeDBItemPrefix):]
	copy(id[:], item[:len(id)])
	field = string(item[len(id):])

	return id, field
}

// fetchInt64 retrieves an integer instance associated with a particular
// database key.
// fetchInt64检索与特定数据库键相关的整数实例。
func (db *nodeDB) fetchInt64(key []byte) int64 {
	blob, err := db.lvl.Get(key, nil)
	if err != nil {
		return 0
	}
	val, read := binary.Varint(blob)
	if read <= 0 {
		return 0
	}
	return val
}

// storeInt64 update a specific database entry to the current time instance as a
// unix timestamp.
// storeInt64将当前时间实例的特定数据库条目更新为unix时间戳。
func (db *nodeDB) storeInt64(key []byte, n int64) error {
	blob := make([]byte, binary.MaxVarintLen64)
	blob = blob[:binary.PutVarint(blob, n)]

	return db.lvl.Put(key, blob, nil)
}

//Node的存储，查询和删除
// node retrieves a node with a given id from the database.
func (db *nodeDB) node(id NodeID) *Node {
	blob, err := db.lvl.Get(makeKey(id, nodeDBDiscoverRoot), nil)
	if err != nil {
		return nil
	}
	node := new(Node)
	if err := rlp.DecodeBytes(blob, node); err != nil {
		log.Error("Failed to decode node RLP", "err", err)
		return nil
	}
	node.sha = crypto.Keccak256Hash(node.ID[:])
	return node
}

// updateNode inserts - potentially overwriting - a node into the peer database.
// updateNode插入 - 可能覆盖 - 一个节点到对等数据库中。
func (db *nodeDB) updateNode(node *Node) error {
	blob, err := rlp.EncodeToBytes(node)
	if err != nil {
		return err
	}
	return db.lvl.Put(makeKey(node.ID, nodeDBDiscoverRoot), blob, nil)
}

// deleteNode deletes all information/keys associated with a node.
// deleteNode删除与节点相关的所有信息/键。
func (db *nodeDB) deleteNode(id NodeID) error {
	deleter := db.lvl.NewIterator(util.BytesPrefix(makeKey(id, "")), nil)
	for deleter.Next() {
		if err := db.lvl.Delete(deleter.Key(), nil); err != nil {
			return err
		}
	}
	return nil
}

// ensureExpirer是确保数据过期机制正在运行的小型辅助方法。
// 如果过期goroutine已经运行，则此方法仅返回。
//目标是在网络成功自行引导后（以防止转储潜在有用的种子节点）开始数据撤出。
// 由于需要大量的开销来精确地追踪第一次成功的收敛，
// 所以当发生适当的情况（即成功的结合）时“确保”正确的状态并且放弃进一步的事件更简单
// ensureExpirer is a small helper method ensuring that the data expiration
// mechanism is running. If the expiration goroutine is already running, this
// method simply returns.
//
// The goal is to start the data evacuation only after the network successfully
// bootstrapped itself (to prevent dumping potentially useful seed nodes). Since
// it would require significant overhead to exactly trace the first successful
// convergence, it's simpler to "ensure" the correct state when an appropriate
// condition occurs (i.e. a successful bonding), and discard further events.
// ensureExpirer方法用来确保expirer方法在运行。 如果expirer已经运行，那么这个方法就直接返回。
// 这个方法设置的目的是为了在网络成功启动后在开始进行数据超时丢弃的工作(以防一些潜在的有用的种子节点被丢弃)。
func (db *nodeDB) ensureExpirer() {
	db.runner.Do(func() { go db.expirer() })
}

//发现者应该在一个围棋程序中开始，并负责循环广告。从数据库中删除陈旧的数据。
// expirer should be started in a go routine, and is responsible for looping ad
// infinitum and dropping stale data from the database.
func (db *nodeDB) expirer() {
	tick := time.NewTicker(nodeDBCleanupCycle)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if err := db.expireNodes(); err != nil {
				log.Error("Failed to expire nodedb items", "err", err)
			}
		case <-db.quit:
			return
		}
	}
}

//这个方法遍历所有的节点，如果某个节点最后接收消息超过指定值，那么就删除这个节点。
// expireNodes iterates over the database and deletes all nodes that have not
// been seen (i.e. received a pong from) for some allotted time.
func (db *nodeDB) expireNodes() error {
	threshold := time.Now().Add(-nodeDBNodeExpiration)

	// Find discovered nodes that are older than the allowance
	it := db.lvl.NewIterator(nil, nil)
	defer it.Release()

	for it.Next() {
		// Skip the item if not a discovery node
		id, field := splitKey(it.Key())
		if field != nodeDBDiscoverRoot {
			continue
		}
		// Skip the node if not expired yet (and not self)
		if !bytes.Equal(id[:], db.self[:]) {
			if seen := db.bondTime(id); seen.After(threshold) {
				continue
			}
		}
		// Otherwise delete all associated information
		db.deleteNode(id)
	}
	return nil
}

//一些状态更新函数
// lastPing retrieves the time of the last ping packet send to a remote node,
// requesting binding.
// lastPing检索最后ping包发送到远程节点的时间，请求绑定。
func (db *nodeDB) lastPing(id NodeID) time.Time {
	return time.Unix(db.fetchInt64(makeKey(id, nodeDBDiscoverPing)), 0)
}

// updateLastPing updates the last time we tried contacting a remote node.
// updateLastPing更新了我们上次尝试联系远程节点的时间。
func (db *nodeDB) updateLastPing(id NodeID, instance time.Time) error {
	return db.storeInt64(makeKey(id, nodeDBDiscoverPing), instance.Unix())
}

// bondTime retrieves the time of the last successful pong from remote node.
// bondTime从远程节点获取最后一次成功的pong的时间。
func (db *nodeDB) bondTime(id NodeID) time.Time {
	return time.Unix(db.fetchInt64(makeKey(id, nodeDBDiscoverPong)), 0)
}

// hasBond reports whether the given node is considered bonded.
// hasBond报告给定节点是否被视为绑定。
func (db *nodeDB) hasBond(id NodeID) bool {
	return time.Since(db.bondTime(id)) < nodeDBNodeExpiration
}

// updateBondTime updates the last pong time of a node.
// updateBondTime更新节点的最后一个pong时间
func (db *nodeDB) updateBondTime(id NodeID, instance time.Time) error {
	return db.storeInt64(makeKey(id, nodeDBDiscoverPong), instance.Unix())
}

// findFails retrieves the number of findnode failures since bonding.
// findFails检索自绑定以来findnode失败的次数。
func (db *nodeDB) findFails(id NodeID) int {
	return int(db.fetchInt64(makeKey(id, nodeDBDiscoverFindFails)))
}

// updateFindFails更新自绑定以来findnode失败的次数。
// updateFindFails updates the number of findnode failures since bonding.
func (db *nodeDB) updateFindFails(id NodeID, fails int) error {
	return db.storeInt64(makeKey(id, nodeDBDiscoverFindFails), int64(fails))
}

//从数据库里面随机挑选合适种子节点
// querySeeds retrieves random nodes to be used as potential seed nodes
// for bootstrapping.
func (db *nodeDB) querySeeds(n int, maxAge time.Duration) []*Node {
	var (
		now   = time.Now()
		nodes = make([]*Node, 0, n)
		it    = db.lvl.NewIterator(nil, nil)
		id    NodeID
	)
	defer it.Release()

seek:
	for seeks := 0; len(nodes) < n && seeks < n*5; seeks++ {
		// Seek to a random entry. The first byte is incremented by a
		// random amount each time in order to increase the likelihood
		// of hitting all existing nodes in very small databases.
		//寻找一个随机条目。 第一个字节每次增加一个随机数，以增加在非常小的数据库中击中所有现有节点的可能性。
		ctr := id[0]
		rand.Read(id[:])
		id[0] = ctr + id[0]%16
		it.Seek(makeKey(id, nodeDBDiscoverRoot))

		n := nextNode(it)
		if n == nil {
			id[0] = 0
			continue seek // iterator exhausted
		}
		if n.ID == db.self {
			continue seek
		}
		if now.Sub(db.bondTime(n.ID)) > maxAge {
			continue seek
		}
		for i := range nodes {
			if nodes[i].ID == n.ID {
				continue seek // duplicate
			}
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// reads the next node record from the iterator, skipping over other
// database entries.
//从迭代器读取下一个节点记录，跳过其他数据库条目。
func nextNode(it iterator.Iterator) *Node {
	for end := false; !end; end = !it.Next() {
		id, field := splitKey(it.Key())
		if field != nodeDBDiscoverRoot {
			continue
		}
		var n Node
		if err := rlp.DecodeBytes(it.Value(), &n); err != nil {
			log.Warn("Failed to decode node RLP", "id", id, "err", err)
			continue
		}
		return &n
	}
	return nil
}

// close flushes and closes the database files.
//关闭刷新并关闭数据库文件。
func (db *nodeDB) close() {
	close(db.quit)
	db.lvl.Close()
}
