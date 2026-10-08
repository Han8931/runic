package client

import (
	"fmt"
	"os"

	"github.com/han/runic/internal/protocol"
)

// Mark sets a pane's self-reported attention state. An empty pane marks every
// live pane in the session.
func Mark(session, pane string, a protocol.Attention, note string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type: protocol.MsgMark,
		Payload: protocol.PayloadMark{
			Session:   session,
			Pane:      pane,
			Attention: a,
			Note:      note,
		},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

// MarkTarget resolves which pane a `ru mark` applies to. Explicit arguments win;
// otherwise it uses the RUNIC_SESSION / RUNIC_PANE pair the daemon puts in every
// pane's environment, which is what lets an agent's hook report its own state
// with no configuration at all.
func MarkTarget(session, pane string) (string, string, error) {
	if session == "" {
		session = os.Getenv("RUNIC_SESSION")
	}
	if pane == "" {
		pane = os.Getenv("RUNIC_PANE")
	}
	if session == "" {
		return "", "", fmt.Errorf("not running inside a runic pane (RUNIC_SESSION unset) — pass --session <name>")
	}
	return session, pane, nil
}
