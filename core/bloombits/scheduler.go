// Copyright 2017 The go-ethereum Authors
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

package bloombits

import (
	"sync"
)

//scheduler是基于section的布隆过滤器的单个bit值检索的调度。
// 除了调度检索操作之外，这个结构还可以对请求进行重复数据删除并缓存结果，
// 从而即使在复杂的过滤情况下也可以将网络/数据库开销降至最低。
// request represents a bloom retrieval task to prioritize and pull from the local
// database or remotely from the network.
//request表示一个bloom检索任务，以便优先从本地数据库中或从网络中剪检索。 section 表示区块段号，每段4096个区块，
// bit代表检索的是布隆过滤器的哪一位(一共有2048位)。这个在之前的(eth-bloombits和filter源码分析.md)中有介绍。
type request struct {
	section uint64 // Section index to retrieve the a bit-vector from
	bit     uint   // Bit index within the section to retrieve the vector of
}

//response当前调度的请求的状态。 没发送一个请求，会生成一个response对象来最终这个请求的状态。
// cached用来缓存这个section的结果。
// response represents the state of a requested bit-vector through a scheduler.
type response struct {
	cached []byte        // Cached bits to dedup multiple requests
	done   chan struct{} // Channel to allow waiting for completion
}

// scheduler handles the scheduling of bloom-filter retrieval operations for
// entire section-batches belonging to a single bloom bit. Beside scheduling the
// retrieval operations, this struct also deduplicates the requests and caches
// the results to minimize network/database overhead even in complex filtering
// scenarios.
type scheduler struct {
	bit       uint                 // Index of the bit in the bloom filter this scheduler is responsible for布隆过滤器的哪一个bit位(0-2047)
	responses map[uint64]*response // Currently pending retrieval requests or already cached responses当前正在进行的请求或者是已经缓存的结果。
	lock      sync.Mutex           // Lock protecting the responses from concurrent access
}

// newScheduler creates a new bloom-filter retrieval scheduler for a specific
// bit index.
func newScheduler(idx uint) *scheduler {
	return &scheduler{
		bit:       idx,
		responses: make(map[uint64]*response),
	}
}

//run方法创建了一个流水线， 从sections channel来接收需要请求的sections，通过done channel来按照请求的顺序返回结果。
// 并发的运行同样的scheduler是可以的，这样会导致任务重复。
// run creates a retrieval pipeline, receiving section indexes from sections and
// returning the results in the same order through the done channel. Concurrent
// runs of the same scheduler are allowed, leading to retrieval task deduplication.
func (s *scheduler) run(sections chan uint64, dist chan *request, done chan []byte, quit chan struct{}, wg *sync.WaitGroup) {
	// Create a forwarder channel between requests and responses of the same size as
	// the distribution channel (since that will block the pipeline anyway)
	// // sections 通道类型 这个是用来传递需要检索的section的通道，输入参数
	// dist     通道类型， 属于输出通道(可能是网络发送或者是本地检索)，往这个通道上发送请求， 然后在done上获取回应。
	// done  用来传递检索结果的通道， 可以理解为返回值通道。
	//在请求和响应之间创建一个与分发通道大小相同的转发器通道（因为这样会阻塞管道）
	pend := make(chan uint64, cap(dist))

	// Start the pipeline schedulers to forward between user -> distributor -> user
	wg.Add(2)
	go s.scheduleRequests(sections, dist, pend, quit, wg)
	go s.scheduleDeliveries(pend, done, quit, wg)
}

//reset用法用来清理之前的所有任何请求。
// reset cleans up any leftovers from previous runs. This is required before a
// restart to ensure the no previously requested but never delivered state will
// cause a lockup.
func (s *scheduler) reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for section, res := range s.responses {
		if res.cached == nil {
			delete(s.responses, section)
		}
	}
}

/*scheduleRequests goroutine从sections接收到section消息
scheduleRequests把接收到的section组装成requtest发送到dist channel，并构建对象response[section]
scheduleRequests把上一部的section发送给pend队列。scheduleDelivers接收到pend消息，阻塞在response[section].done上面
外部调用deliver方法，把seciton的request请求结果写入response[section].cached.并关闭response[section].done channel
scheduleDelivers接收到response[section].done 信息。 把response[section].cached 发送到done channel*/

// scheduleRequests reads section retrieval requests from the input channel,
// deduplicates the stream and pushes unique retrieval tasks into the distribution
// channel for a database or network layer to honour.
func (s *scheduler) scheduleRequests(reqs chan uint64, dist chan *request, pend chan uint64, quit chan struct{}, wg *sync.WaitGroup) {
	// Clean up the goroutine and pipeline when done
	defer wg.Done()
	defer close(pend)

	// Keep reading and scheduling section requests
	for {
		select {
		case <-quit:
			return

		case section, ok := <-reqs:
			// New section retrieval requested
			if !ok {
				return
			}
			// Deduplicate retrieval requests
			unique := false

			s.lock.Lock()
			if s.responses[section] == nil {
				s.responses[section] = &response{
					done: make(chan struct{}),
				}
				unique = true
			}
			s.lock.Unlock()

			// Schedule the section for retrieval and notify the deliverer to expect this section
			if unique {
				select {
				case <-quit:
					return
				case dist <- &request{bit: s.bit, section: section}:
				}
			}
			select {
			case <-quit:
				return
			case pend <- section:
			}
		}
	}
}

// scheduleDeliveries reads section acceptance notifications and waits for them
// to be delivered, pushing them into the output data buffer.
func (s *scheduler) scheduleDeliveries(pend chan uint64, done chan []byte, quit chan struct{}, wg *sync.WaitGroup) {
	// Clean up the goroutine and pipeline when done
	defer wg.Done()
	defer close(done)

	// Keep reading notifications and scheduling deliveries
	for {
		select {
		case <-quit:
			return

		case idx, ok := <-pend:
			// New section retrieval pending
			if !ok {
				return
			}
			// Wait until the request is honoured
			s.lock.Lock()
			res := s.responses[idx]
			s.lock.Unlock()

			select {
			case <-quit:
				return
			case <-res.done:
			}
			// Deliver the result
			select {
			case <-quit:
				return
			case done <- res.cached:
			}
		}
	}
}

// deliver is called by the request distributor when a reply to a request arrives.
func (s *scheduler) deliver(sections []uint64, data [][]byte) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for i, section := range sections {
		if res := s.responses[section]; res != nil && res.cached == nil { // Avoid non-requests and double deliveries
			res.cached = data[i]
			close(res.done)
		}
	}
}
