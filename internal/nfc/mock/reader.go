// Package mock provides deterministic hardware-free NFC test doubles.
package mock

import (
	"context"
	"sync"

	"github.com/BennyThink/NFCX/internal/nfc"
)

// ReaderFuncs can override individual mock operations. Nil functions use the
// safe default behaviour documented on NewReader.
type ReaderFuncs struct {
	Open              func(context.Context, string) error
	Close             func() error
	CardInfo          func(context.Context) (nfc.CardInfo, error)
	Authenticate      func(context.Context, byte, nfc.KeyType, nfc.Key) error
	ReadBlock         func(context.Context, byte) ([nfc.BlockSize]byte, error)
	WriteBlock        func(context.Context, byte, [nfc.BlockSize]byte) error
	WriteTrailer      func(context.Context, byte, [nfc.BlockSize]byte) error
	WriteManufacturer func(context.Context, [nfc.BlockSize]byte) error
}

// Reader is a thread-safe, configurable implementation of nfc.Reader.
type Reader struct {
	opMu       sync.Mutex
	mu         sync.Mutex
	funcs      ReaderFuncs
	open       bool
	connString string
	openCount  int
	closeCount int
	card       nfc.CardInfo
	cardErr    error
}

// NewReader creates a closed mock with safe default behaviour.
func NewReader() *Reader {
	return NewReaderWithFuncs(ReaderFuncs{})
}

// NewReaderWithFuncs creates a closed mock with selected operation overrides.
// By default Open and Close succeed, CardInfo reports no card, and Classic
// operations report unsupported until a test configures their functions.
func NewReaderWithFuncs(funcs ReaderFuncs) *Reader {
	return &Reader{funcs: funcs, cardErr: nfc.ErrNoCard}
}

// SetCardResult changes the result returned by the default CardInfo function.
func (r *Reader) SetCardResult(card nfc.CardInfo, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.card = card.Clone()
	r.cardErr = err
}

// State returns a snapshot useful to tests of higher application layers.
func (r *Reader) State() (open bool, connString string, openCount, closeCount int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.open, r.connString, r.openCount, r.closeCount
}

func (r *Reader) Open(ctx context.Context, connString string) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "open"); err != nil {
		return err
	}
	if r.isOpen() {
		return nfc.NewError("open", nfc.CodeBusy, "reader is already open", nil)
	}
	if r.funcs.Open != nil {
		if err := r.funcs.Open(ctx, connString); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.open = true
	r.connString = connString
	r.openCount++
	return nil
}

func (r *Reader) Close() error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if !r.isOpen() {
		return nil
	}
	if r.funcs.Close != nil {
		if err := r.funcs.Close(); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.open {
		r.closeCount++
	}
	r.open = false
	return nil
}

func (r *Reader) CardInfo(ctx context.Context) (nfc.CardInfo, error) {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "card info"); err != nil {
		return nfc.CardInfo{}, err
	}
	if !r.isOpen() {
		return nfc.CardInfo{}, nfc.NewError("card info", nfc.CodeNotOpen, "", nil)
	}
	if r.funcs.CardInfo != nil {
		card, err := r.funcs.CardInfo(ctx)
		return card.Clone(), err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.card.Clone(), r.cardErr
}

func (r *Reader) Authenticate(ctx context.Context, block byte, keyType nfc.KeyType, key nfc.Key) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "authenticate"); err != nil {
		return err
	}
	if !r.isOpen() {
		return nfc.NewError("authenticate", nfc.CodeNotOpen, "", nil)
	}
	if r.funcs.Authenticate != nil {
		return r.funcs.Authenticate(ctx, block, keyType, key)
	}
	return nfc.NewError("authenticate", nfc.CodeUnsupported, "mock authentication is not configured", nil)
}

func (r *Reader) ReadBlock(ctx context.Context, block byte) ([nfc.BlockSize]byte, error) {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "read block"); err != nil {
		return [nfc.BlockSize]byte{}, err
	}
	if !r.isOpen() {
		return [nfc.BlockSize]byte{}, nfc.NewError("read block", nfc.CodeNotOpen, "", nil)
	}
	if r.funcs.ReadBlock != nil {
		return r.funcs.ReadBlock(ctx, block)
	}
	return [nfc.BlockSize]byte{}, nfc.NewError("read block", nfc.CodeUnsupported, "mock block read is not configured", nil)
}

func (r *Reader) WriteBlock(ctx context.Context, block byte, data [nfc.BlockSize]byte) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "write block"); err != nil {
		return err
	}
	if !r.isOpen() {
		return nfc.NewError("write block", nfc.CodeNotOpen, "", nil)
	}
	if r.funcs.WriteBlock != nil {
		return r.funcs.WriteBlock(ctx, block, data)
	}
	return nfc.NewError("write block", nfc.CodeUnsupported, "mock block write is not configured", nil)
}

func (r *Reader) WriteSectorTrailer(ctx context.Context, block byte, data [nfc.BlockSize]byte) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "write sector trailer"); err != nil {
		return err
	}
	if !r.isOpen() {
		return nfc.NewError("write sector trailer", nfc.CodeNotOpen, "", nil)
	}
	if r.funcs.WriteTrailer != nil {
		return r.funcs.WriteTrailer(ctx, block, data)
	}
	return nfc.NewError("write sector trailer", nfc.CodeUnsupported, "mock trailer write is not configured", nil)
}

func (r *Reader) WriteManufacturerBlock(ctx context.Context, data [nfc.BlockSize]byte) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := contextError(ctx, "write manufacturer block"); err != nil {
		return err
	}
	if !r.isOpen() {
		return nfc.NewError("write manufacturer block", nfc.CodeNotOpen, "", nil)
	}
	if r.funcs.WriteManufacturer != nil {
		return r.funcs.WriteManufacturer(ctx, data)
	}
	return nfc.NewError("write manufacturer block", nfc.CodeUnsupported, "mock manufacturer block write is not configured", nil)
}

func (r *Reader) isOpen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.open
}

func contextError(ctx context.Context, op string) error {
	if ctx == nil {
		return nfc.NewError(op, nfc.CodeInvalidArgument, "nil context", nil)
	}
	if err := ctx.Err(); err != nil {
		code := nfc.CodeCanceled
		if err == context.DeadlineExceeded {
			code = nfc.CodeTimeout
		}
		return nfc.NewError(op, code, "", err)
	}
	return nil
}

var _ nfc.Reader = (*Reader)(nil)
var _ nfc.ClassicTrailerWriter = (*Reader)(nil)
var _ nfc.ClassicManufacturerWriter = (*Reader)(nil)
