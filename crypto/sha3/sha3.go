// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sha3

//海绵方向表示方向字节在海绵中流动。
// spongeDirection indicates the direction bytes are flowing through the sponge.
type spongeDirection int

const (
	// spongeAbsorbing indicates that the sponge is absorbing input.// spongeAbsorbing表示海绵吸收输入。
	spongeAbsorbing spongeDirection = iota
	// spongeSqueezing indicates that the sponge is being squeezed.// spongeSqueezing表示海绵被挤压。
	spongeSqueezing
)

const (
	//maxRate是内部缓冲区的最大大小。握手-256/当前需要最大的缓冲区。
	// maxRate is the maximum size of the internal buffer. SHAKE-256
	// currently needs the largest buffer.
	maxRate = 168
)

type state struct {
	// Generic sponge components.//通用海绵组件
	a    [25]uint64 // main state of the hash//哈希的主要状态
	buf  []byte     // points into storage//指向存储
	rate int        // the number of bytes of state to use//要使用的状态字节数
	// dsbyte包含“域分隔”位和填充的第一位。 [1]的第6.1和6.2节将输出分开
	// SHA-3和SHAKE通过在消息中附加位串来实现。
	//使用little-endian位排序约定，SHA-3为“01”，SHAKE为“1111”，或00000010b和00001111b。 然后应用来自5.1节的填充规则以将消息填充到速率的倍数，其涉及添加“1”位，零或更多“0”位以及最后的“1”位。 我们将填充中的第一个“1”位合并为dsbyte，
	//给出00000110b（0x06）和00011111b（0x1f）。 [1] http://csrc.nist.gov/publications/drafts/fips-202/fips_202_draft.pdf
	//“草案FIPS 202：SHA-3标准：基于排列的哈希和
	//可扩展输出功能（2014年5月）“
	// dsbyte contains the "domain separation" bits and the first bit of
	// the padding. Sections 6.1 and 6.2 of [1] separate the outputs of the
	// SHA-3 and SHAKE functions by appending bitstrings to the message.
	// Using a little-endian bit-ordering convention, these are "01" for SHA-3
	// and "1111" for SHAKE, or 00000010b and 00001111b, respectively. Then the
	// padding rule from section 5.1 is applied to pad the message to a multiple
	// of the rate, which involves adding a "1" bit, zero or more "0" bits, and
	// a final "1" bit. We merge the first "1" bit from the padding into dsbyte,
	// giving 00000110b (0x06) and 00011111b (0x1f).
	// [1] http://csrc.nist.gov/publications/drafts/fips-202/fips_202_draft.pdf
	//     "Draft FIPS 202: SHA-3 Standard: Permutation-Based Hash and
	//      Extendable-Output Functions (May 2014)"
	dsbyte  byte
	storage [maxRate]byte
	//特定于SHA-3和SHAKE。
	// Specific to SHA-3 and SHAKE.
	outputLen int             // the default output size in bytes默认输出大小（以字节为单位）
	state     spongeDirection // whether the sponge is absorbing or squeezing海绵是吸收还是挤压
}

// BlockSize returns the rate of sponge underlying this hash function.
func (d *state) BlockSize() int { return d.rate }

// Size returns the output size of the hash function in bytes.
func (d *state) Size() int { return d.outputLen }

// Reset clears the internal state by zeroing the sponge state and
// the byte buffer, and setting Sponge.state to absorbing.
func (d *state) Reset() {
	// Zero the permutation's state.
	for i := range d.a {
		d.a[i] = 0
	}
	d.state = spongeAbsorbing
	d.buf = d.storage[:0]
}

func (d *state) clone() *state {
	ret := *d
	if ret.state == spongeAbsorbing {
		ret.buf = ret.storage[:len(ret.buf)]
	} else {
		ret.buf = ret.storage[d.rate-cap(d.buf) : d.rate]
	}

	return &ret
}

// permute applies the KeccakF-1600 permutation. It handles
// any input-output buffering.
func (d *state) permute() {
	switch d.state {
	case spongeAbsorbing:
		// If we're absorbing, we need to xor the input into the state
		// before applying the permutation.
		xorIn(d, d.buf)
		d.buf = d.storage[:0]
		keccakF1600(&d.a)
	case spongeSqueezing:
		// If we're squeezing, we need to apply the permutatin before
		// copying more output.
		keccakF1600(&d.a)
		d.buf = d.storage[:d.rate]
		copyOut(d, d.buf)
	}
}

// pads appends the domain separation bits in dsbyte, applies
// the multi-bitrate 10..1 padding rule, and permutes the state.
func (d *state) padAndPermute(dsbyte byte) {
	if d.buf == nil {
		d.buf = d.storage[:0]
	}
	// Pad with this instance's domain-separator bits. We know that there's
	// at least one byte of space in d.buf because, if it were full,
	// permute would have been called to empty it. dsbyte also contains the
	// first one bit for the padding. See the comment in the state struct.
	d.buf = append(d.buf, dsbyte)
	zerosStart := len(d.buf)
	d.buf = d.storage[:d.rate]
	for i := zerosStart; i < d.rate; i++ {
		d.buf[i] = 0
	}
	// This adds the final one bit for the padding. Because of the way that
	// bits are numbered from the LSB upwards, the final bit is the MSB of
	// the last byte.
	d.buf[d.rate-1] ^= 0x80
	// Apply the permutation
	d.permute()
	d.state = spongeSqueezing
	d.buf = d.storage[:d.rate]
	copyOut(d, d.buf)
}

// Write会将更多数据吸收到哈希状态。 它会产生错误
//如果在写入后将更多数据写入ShakeHash
// Write absorbs more data into the hash's state. It produces an error
// if more data is written to the ShakeHash after writing
func (d *state) Write(p []byte) (written int, err error) {
	if d.state != spongeAbsorbing {
		panic("sha3: write to sponge after read")
	}
	if d.buf == nil {
		d.buf = d.storage[:0]
	}
	written = len(p)

	for len(p) > 0 {
		if len(d.buf) == 0 && len(p) >= d.rate {
			// The fast path; absorb a full "rate" bytes of input and apply the permutation.
			xorIn(d, p[:d.rate])
			p = p[d.rate:]
			keccakF1600(&d.a)
		} else {
			// The slow path; buffer the input until we can fill the sponge, and then xor it in.
			todo := d.rate - len(d.buf)
			if todo > len(p) {
				todo = len(p)
			}
			d.buf = append(d.buf, p[:todo]...)
			p = p[todo:]

			// If the sponge is full, apply the permutation.
			if len(d.buf) == d.rate {
				d.permute()
			}
		}
	}

	return
}

// Read squeezes an arbitrary number of bytes from the sponge.
func (d *state) Read(out []byte) (n int, err error) {
	// If we're still absorbing, pad and apply the permutation.
	if d.state == spongeAbsorbing {
		d.padAndPermute(d.dsbyte)
	}

	n = len(out)

	// Now, do the squeezing.
	for len(out) > 0 {
		n := copy(out, d.buf)
		d.buf = d.buf[n:]
		out = out[n:]

		// Apply the permutation if we've squeezed the sponge dry.
		if len(d.buf) == 0 {
			d.permute()
		}
	}

	return
}

// Sum applies padding to the hash state and then squeezes out the desired
// number of output bytes.
func (d *state) Sum(in []byte) []byte {
	// Make a copy of the original hash so that caller can keep writing
	// and summing.
	dup := d.clone()
	hash := make([]byte, dup.outputLen)
	dup.Read(hash)
	return append(in, hash...)
}
