package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BennyThink/NFCX/internal/keys"
	"github.com/BennyThink/NFCX/internal/nfc"
	"github.com/BennyThink/NFCX/internal/nfc/mock"
	"github.com/BennyThink/NFCX/internal/workflow"
)

func TestKeyScanFindsBothTypesAndPromotesSuccess(t *testing.T) {
	card := classic1KCard(1)
	store := keys.NewStore()
	winner := nfc.Key{1, 2, 3, 4, 5, 6}
	record, _, err := store.Add(winner, keys.SourceUserInput)
	if err != nil {
		t.Fatal(err)
	}
	attempts := make([]nfc.Key, 0)
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, _ byte, _ nfc.KeyType, key nfc.Key) error {
			attempts = append(attempts, key)
			if key == winner {
				return nil
			}
			return nfc.ErrAuthenticationFailed
		},
	})
	result, err := workflow.NewKeyScanService(directExecutor{reader: reader}, store).Scan(context.Background(), workflow.KeyScanRequest{Card: card}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 32 {
		t.Fatalf("matches = %d, want 32", len(result.Matches))
	}
	if result.Matches[0].KeyID != record.ID {
		t.Fatalf("first match = %+v", result.Matches[0])
	}
	// Once sector 0 succeeds, both slots in later sectors should try the
	// promoted key first rather than restarting the built-in list.
	if len(attempts) >= 4 && (attempts[len(attempts)-1] != winner || attempts[len(attempts)-2] != winner) {
		t.Fatalf("successful key was not promoted: %x", attempts[len(attempts)-2:])
	}
}

func TestKeyScanCancellationKeepsConfirmedResults(t *testing.T) {
	card := classic1KCard(1)
	store := keys.NewStore()
	winner := nfc.Key{1, 2, 3, 4, 5, 6}
	if _, _, err := store.Add(winner, keys.SourceUserInput); err != nil {
		t.Fatal(err)
	}
	reader := openMockReader(t, mock.ReaderFuncs{
		CardInfo: func(context.Context) (nfc.CardInfo, error) { return card, nil },
		Authenticate: func(_ context.Context, _ byte, _ nfc.KeyType, key nfc.Key) error {
			if key == winner {
				return nil
			}
			return nfc.ErrAuthenticationFailed
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	result, err := workflow.NewKeyScanService(directExecutor{reader: reader}, store).Scan(ctx, workflow.KeyScanRequest{Card: card}, func(progress workflow.KeyScanProgress) {
		if progress.Matched {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) || !result.Cancelled || len(result.Matches) != 1 {
		t.Fatalf("cancelled result/error = %+v / %v", result, err)
	}
	if matches := store.Verified(keys.CardID(card)); len(matches) != 1 {
		t.Fatalf("persisted matches = %+v", matches)
	}
}
