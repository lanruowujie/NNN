// Package attack runs bundled external engines behind a hardware-independent
// contract. It owns process execution, logging, cancellation, timeouts and
// result validation; concrete adapters own only their stable arguments and
// result formats.
package attack
