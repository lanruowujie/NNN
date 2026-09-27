package nfc

import "bytes"

// CardType is a conservative inference from ISO/IEC 14443A selection data.
// It is a hint for the UI, never proof of a card's exact product or capacity.
type CardType string

const (
	CardTypeUnknown         CardType = "unknown"
	CardTypeMIFAREMini      CardType = "mifare_classic_mini"
	CardTypeMIFAREClassic1K CardType = "mifare_classic_1k"
	CardTypeMIFAREClassic4K CardType = "mifare_classic_4k"
)

// InferCardType recognizes only the unambiguous Classic-family SAK values
// NFCX currently needs. All other values deliberately remain unknown.
func InferCardType(card CardInfo) CardType {
	switch card.SAK &^ 0x04 { // Ignore the ISO14443A cascade bit if a backend retains it.
	case 0x09:
		return CardTypeMIFAREMini
	case 0x08:
		return CardTypeMIFAREClassic1K
	case 0x18:
		return CardTypeMIFAREClassic4K
	default:
		return CardTypeUnknown
	}
}

// SameCard reports whether two selections identify the same ISO14443A card.
// ATQA and SAK are included so a UID collision cannot silently change the
// operation's assumed card type or capacity.
func SameCard(left, right CardInfo) bool {
	return bytes.Equal(left.UID, right.UID) && left.ATQA == right.ATQA && left.SAK == right.SAK
}
