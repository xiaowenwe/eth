// Copyright 2016 The go-ethereum Authors
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

package event

import (
	"context"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/mclock"
)

//订阅代表一系列事件。 事件的载体通常是一个通道，但不是界面的一部分。
//订阅可能会在建立时失败。 通过错误报告故障
//频道。 如果订阅存在问题（例如，传递事件的网络连接已关闭），它将收到一个值。 只有一个值会被发送。
//当订阅成功结束时，错误频道关闭（即， 事件源已关闭）。 取消订阅时也会关闭。
//取消订阅方法取消发送事件。 您必须在所有情况下调用取消订阅，以确保与订阅相关的资源被释放。
// 它可以被称为任意次数。
// Subscription represents a stream of events. The carrier of the events is typically a
// channel, but isn't part of the interface.
//
// Subscriptions can fail while established. Failures are reported through an error
// channel. It receives a value if there is an issue with the subscription (e.g. the
// network connection delivering the events has been closed). Only one value will ever be
// sent.
//
// The error channel is closed when the subscription ends successfully (i.e. when the
// source of events is closed). It is also closed when Unsubscribe is called.
//
// The Unsubscribe method cancels the sending of events. You must call Unsubscribe in all
// cases to ensure that resources related to the subscription are released. It can be
// called any number of times.
type Subscription interface {
	Err() <-chan error // returns the error channel
	Unsubscribe()      // cancels sending of events, closing the error channel//取消发送事件，关闭错误频道
}

// NewSubscription在新的goroutine中运行生产者函数作为订阅。 调用Unsubscribe时，将关闭给予生产者的通道。 如果fn返回错误，则会在订阅的错误通道上发送。
// NewSubscription runs a producer function as a subscription in a new goroutine. The
// channel given to the producer is closed when Unsubscribe is called. If fn returns an
// error, it is sent on the subscription's error channel.
func NewSubscription(producer func(<-chan struct{}) error) Subscription {
	s := &funcSub{unsub: make(chan struct{}), err: make(chan error, 1)}
	go func() {
		defer close(s.err)
		err := producer(s.unsub)
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.unsubscribed {
			if err != nil {
				s.err <- err
			}
			s.unsubscribed = true
		}
	}()
	return s
}

type funcSub struct {
	unsub        chan struct{}
	err          chan error
	mu           sync.Mutex
	unsubscribed bool
}

func (s *funcSub) Unsubscribe() {
	s.mu.Lock()
	if s.unsubscribed {
		s.mu.Unlock()
		return
	}
	s.unsubscribed = true
	close(s.unsub)
	s.mu.Unlock()
	// Wait for producer shutdown.
	<-s.err
}

func (s *funcSub) Err() <-chan error {
	return s.err
}

//重复重新订阅fn以保持订阅。 订阅建立后，Resubscribe会等待它失败并再次调用fn。 这个 重复过程，直到调用Unsubscribe或活动订阅成功结束。
// Resubscribe在fn调用之间应用退避。 呼叫之间的时间是适应的 基于错误率，但永远不会超过backoffMax。
// Resubscribe calls fn repeatedly to keep a subscription established. When the
// subscription is established, Resubscribe waits for it to fail and calls fn again. This
// process repeats until Unsubscribe is called or the active subscription ends
// successfully.
//
// Resubscribe applies backoff between calls to fn. The time between calls is adapted
// based on the error rate, but will never exceed backoffMax.
func Resubscribe(backoffMax time.Duration, fn ResubscribeFunc) Subscription {
	s := &resubscribeSub{
		waitTime:   backoffMax / 10,
		backoffMax: backoffMax,
		fn:         fn,
		err:        make(chan error),
		unsub:      make(chan struct{}),
	}
	go s.loop()
	return s
}

// A ResubscribeFunc attempts to establish a subscription.
type ResubscribeFunc func(context.Context) (Subscription, error)

type resubscribeSub struct {
	fn                   ResubscribeFunc
	err                  chan error
	unsub                chan struct{}
	unsubOnce            sync.Once
	lastTry              mclock.AbsTime
	waitTime, backoffMax time.Duration
}

func (s *resubscribeSub) Unsubscribe() {
	s.unsubOnce.Do(func() {
		s.unsub <- struct{}{}
		<-s.err
	})
}

func (s *resubscribeSub) Err() <-chan error {
	return s.err
}

func (s *resubscribeSub) loop() {
	defer close(s.err)
	var done bool
	for !done {
		sub := s.subscribe()
		if sub == nil {
			break
		}
		done = s.waitForError(sub)
		sub.Unsubscribe()
	}
}

func (s *resubscribeSub) subscribe() Subscription {
	subscribed := make(chan error)
	var sub Subscription
retry:
	for {
		s.lastTry = mclock.Now()
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			rsub, err := s.fn(ctx)
			sub = rsub
			subscribed <- err
		}()
		select {
		case err := <-subscribed:
			cancel()
			if err != nil {
				// Subscribing failed, wait before launching the next try.
				if s.backoffWait() {
					return nil
				}
				continue retry
			}
			if sub == nil {
				panic("event: ResubscribeFunc returned nil subscription and no error")
			}
			return sub
		case <-s.unsub:
			cancel()
			return nil
		}
	}
}

func (s *resubscribeSub) waitForError(sub Subscription) bool {
	defer sub.Unsubscribe()
	select {
	case err := <-sub.Err():
		return err == nil
	case <-s.unsub:
		return true
	}
}

func (s *resubscribeSub) backoffWait() bool {
	if time.Duration(mclock.Now()-s.lastTry) > s.backoffMax {
		s.waitTime = s.backoffMax / 10
	} else {
		s.waitTime *= 2
		if s.waitTime > s.backoffMax {
			s.waitTime = s.backoffMax
		}
	}

	t := time.NewTimer(s.waitTime)
	defer t.Stop()
	select {
	case <-t.C:
		return false
	case <-s.unsub:
		return true
	}
}

// SubscriptionScope提供了一次性取消订阅多个订阅的功能。
//对于处理多个订阅的代码，可以使用范围来方便地通过一次调用来取消订阅它们。 该示例演示了在较大程序中的典型用法。
//零值已准备好使用。
// SubscriptionScope provides a facility to unsubscribe multiple subscriptions at once.
//
// For code that handle more than one subscription, a scope can be used to conveniently
// unsubscribe all of them with a single call. The example demonstrates a typical use in a
// larger program.
//
// The zero value is ready to use.
type SubscriptionScope struct {
	mu     sync.Mutex
	subs   map[*scopeSub]struct{}
	closed bool
}

type scopeSub struct {
	sc *SubscriptionScope
	s  Subscription
}

//跟踪开始跟踪订阅。 如果范围已关闭，则Track返回nil。 返回的订阅是一个包装器。 取消订阅包装器会将其从范围中删除。
// Track starts tracking a subscription. If the scope is closed, Track returns nil. The
// returned subscription is a wrapper. Unsubscribing the wrapper removes it from the
// scope.
func (sc *SubscriptionScope) Track(s Subscription) Subscription {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.closed {
		return nil
	}
	if sc.subs == nil {
		sc.subs = make(map[*scopeSub]struct{})
	}
	ss := &scopeSub{sc, s}
	sc.subs[ss] = struct{}{}
	return ss
}

// Close calls Unsubscribe on all tracked subscriptions and prevents further additions to
// the tracked set. Calls to Track after Close return nil.
func (sc *SubscriptionScope) Close() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.closed {
		return
	}
	sc.closed = true
	for s := range sc.subs {
		s.s.Unsubscribe()
	}
	sc.subs = nil
}

// Count returns the number of tracked subscriptions.
// It is meant to be used for debugging.
func (sc *SubscriptionScope) Count() int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return len(sc.subs)
}

func (s *scopeSub) Unsubscribe() {
	s.s.Unsubscribe()
	s.sc.mu.Lock()
	defer s.sc.mu.Unlock()
	delete(s.sc.subs, s)
}

func (s *scopeSub) Err() <-chan error {
	return s.s.Err()
}
