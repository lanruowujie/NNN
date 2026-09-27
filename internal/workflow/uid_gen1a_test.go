package workflow

import (
	"strings"
	"testing"
)

func TestDescribeGen1AResultDistinguishesUnlockStages(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "first command rejected", output: "Warning: Unlock command [1/2]: failed / not acknowledged.", want: "7-bit 0x40"},
		{name: "second command rejected", output: "Warning: Unlock command [2/2]: failed / not acknowledged.", want: "0x43"},
		{name: "unlocked", output: "Card unlocked", want: "已接受后门解锁"},
		{name: "unknown", output: "NFC reader opened", want: "未报告明确"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := describeGen1AResult([]byte(test.output), nil); !strings.Contains(got, test.want) {
				t.Fatalf("describeGen1AResult() = %q, want substring %q", got, test.want)
			}
		})
	}
}
