// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"golang.org/x/crypto/argon2"
)

// MaxBlockHeaderPayload is the maximum number of bytes a block header can be.
// Version 4 bytes + Timestamp 4 bytes + Bits 4 bytes + Nonce 4 bytes +
// PrevBlock and MerkleRoot hashes.
const MaxBlockHeaderPayload = 16 + (chainhash.HashSize * 2)

// BlockHeader defines information about a block and is used in the bitcoin
// block (MsgBlock) and headers (MsgHeaders) messages.
type BlockHeader struct {
	// Version of the block.  This is not the same as the protocol version.
	Version int32

	// Hash of the previous block header in the block chain.
	PrevBlock chainhash.Hash

	// Merkle tree reference to hash of all transactions for the block.
	MerkleRoot chainhash.Hash

	// Time the block was created.  This is, unfortunately, encoded as a
	// uint32 on the wire and therefore is limited to 2106.
	Timestamp time.Time

	// Difficulty target for the block.
	Bits uint32

	// Nonce used to generate the block.
	Nonce uint32
}

// blockHeaderLen is a constant that represents the number of bytes for a block
// header.
const blockHeaderLen = 80

// SLUG proof-of-work parameters (change set 1). The block header hash
// is an Argon2id digest instead of Bitcoin's double SHA256. The settled
// parameters (see ARGON2ID_PARAMS.md in the SLUG design docs) are a
// 256 MB memory arena, a single pass, and a single lane per attempt;
// miners parallelize across attempts, not within one.
const (
	// powArgon2Time is the Argon2id time cost (number of passes).
	powArgon2Time = 1

	// powArgon2Memory is the Argon2id memory cost in kibibytes, as the
	// argon2 API takes it: 256 * 1024 KiB = 256 MB.
	powArgon2Memory = 256 * 1024

	// powArgon2Threads is the Argon2id parallelism (lanes) per attempt.
	powArgon2Threads = 1

	// powArgon2KeyLen is the Argon2id output length. At 32 bytes the
	// digest fits the existing chainhash.Hash type, so no block ID,
	// database key, or wire format changes size.
	powArgon2KeyLen = 32
)

// BlockHash computes the block identifier hash for the given block header.
//
// SLUG hash construction: the header is serialized in the standard
// 80-byte wire format, and the block hash is
//
//	argon2.IDKey(password = serializedHeader, salt = serializedHeader,
//	             time = 1, memory = 256*1024 KiB, threads = 1, keyLen = 32)
//
// The serialized header serves as both the Argon2id password and its
// salt. There is no per-block or per-network salt to store or transmit:
// any node can recompute the digest from the header alone, which is
// exactly the property the old double SHA256 construction had. Only
// this block header hash uses Argon2id; transaction IDs, merkle roots,
// address checksums, and signature hashes remain double SHA256.
func (h *BlockHeader) BlockHash() chainhash.Hash {
	// writeBlockHeader cannot fail when the writer is a bytes.Buffer.
	var buf bytes.Buffer
	buf.Grow(blockHeaderLen)
	_ = writeBlockHeader(&buf, 0, h)
	serialized := buf.Bytes()

	digest := argon2.IDKey(serialized, serialized, powArgon2Time,
		powArgon2Memory, powArgon2Threads, powArgon2KeyLen)

	var hash chainhash.Hash
	copy(hash[:], digest)
	return hash
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
// See Deserialize for decoding block headers stored to disk, such as in a
// database, as opposed to decoding block headers from the wire.
func (h *BlockHeader) BtcDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	return readBlockHeader(r, pver, h)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
// See Serialize for encoding block headers to be stored to disk, such as in a
// database, as opposed to encoding block headers for the wire.
func (h *BlockHeader) BtcEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	return writeBlockHeader(w, pver, h)
}

// Deserialize decodes a block header from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Version field.
func (h *BlockHeader) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of readBlockHeader.
	return readBlockHeader(r, 0, h)
}

// Serialize encodes a block header from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Version field.
func (h *BlockHeader) Serialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of writeBlockHeader.
	return writeBlockHeader(w, 0, h)
}

// NewBlockHeader returns a new BlockHeader using the provided version, previous
// block hash, merkle root hash, difficulty bits, and nonce used to generate the
// block with defaults for the remaining fields.
func NewBlockHeader(version int32, prevHash, merkleRootHash *chainhash.Hash,
	bits uint32, nonce uint32) *BlockHeader {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &BlockHeader{
		Version:    version,
		PrevBlock:  *prevHash,
		MerkleRoot: *merkleRootHash,
		Timestamp:  time.Unix(time.Now().Unix(), 0),
		Bits:       bits,
		Nonce:      nonce,
	}
}

// readBlockHeader reads a bitcoin block header from r.  See Deserialize for
// decoding block headers stored to disk, such as in a database, as opposed to
// decoding from the wire.
//
// DEPRECATED: Use readBlockHeaderBuf instead.
func readBlockHeader(r io.Reader, pver uint32, bh *BlockHeader) error {
	buf := binarySerializer.Borrow()
	err := readBlockHeaderBuf(r, pver, bh, buf)
	binarySerializer.Return(buf)
	return err
}

// readBlockHeaderBuf reads a bitcoin block header from r.  See Deserialize for
// decoding block headers stored to disk, such as in a database, as opposed to
// decoding from the wire.
//
// If b is non-nil, the provided buffer will be used for serializing small
// values.  Otherwise a buffer will be drawn from the binarySerializer's pool
// and return when the method finishes.
//
// NOTE: b MUST either be nil or at least an 8-byte slice.
func readBlockHeaderBuf(r io.Reader, pver uint32, bh *BlockHeader,
	buf []byte) error {

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return err
	}
	bh.Version = int32(littleEndian.Uint32(buf[:4]))

	if _, err := io.ReadFull(r, bh.PrevBlock[:]); err != nil {
		return err
	}

	if _, err := io.ReadFull(r, bh.MerkleRoot[:]); err != nil {
		return err
	}

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return err
	}
	bh.Timestamp = time.Unix(int64(littleEndian.Uint32(buf[:4])), 0)

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return err
	}
	bh.Bits = littleEndian.Uint32(buf[:4])

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return err
	}
	bh.Nonce = littleEndian.Uint32(buf[:4])

	return nil
}

// writeBlockHeader writes a bitcoin block header to w.  See Serialize for
// encoding block headers to be stored to disk, such as in a database, as
// opposed to encoding for the wire.
//
// DEPRECATED: Use writeBlockHeaderBuf instead.
func writeBlockHeader(w io.Writer, pver uint32, bh *BlockHeader) error {
	buf := binarySerializer.Borrow()
	err := writeBlockHeaderBuf(w, pver, bh, buf)
	binarySerializer.Return(buf)
	return err
}

// writeBlockHeaderBuf writes a bitcoin block header to w.  See Serialize for
// encoding block headers to be stored to disk, such as in a database, as
// opposed to encoding for the wire.
//
// If b is non-nil, the provided buffer will be used for serializing small
// values.  Otherwise a buffer will be drawn from the binarySerializer's pool
// and return when the method finishes.
//
// NOTE: b MUST either be nil or at least an 8-byte slice.
func writeBlockHeaderBuf(w io.Writer, pver uint32, bh *BlockHeader,
	buf []byte) error {

	littleEndian.PutUint32(buf[:4], uint32(bh.Version))
	if _, err := w.Write(buf[:4]); err != nil {
		return err
	}

	if _, err := w.Write(bh.PrevBlock[:]); err != nil {
		return err
	}

	if _, err := w.Write(bh.MerkleRoot[:]); err != nil {
		return err
	}

	littleEndian.PutUint32(buf[:4], uint32(bh.Timestamp.Unix()))
	if _, err := w.Write(buf[:4]); err != nil {
		return err
	}

	littleEndian.PutUint32(buf[:4], bh.Bits)
	if _, err := w.Write(buf[:4]); err != nil {
		return err
	}

	littleEndian.PutUint32(buf[:4], bh.Nonce)
	if _, err := w.Write(buf[:4]); err != nil {
		return err
	}

	return nil
}
