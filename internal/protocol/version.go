package protocol

// ProtocolVersion gates client/daemon compatibility: a client refuses to talk
// to a daemon speaking a different version rather than guess at the wire.
// Bump it whenever a message type or payload shape changes.
//
// 10: MsgMark + per-pane attention state (Attention on SessionInfo/TerminalInfo).
// 11: acknowledged-but-unresolved alerts (Seen on TerminalInfo, AttentionSeen on
// SessionInfo) — attaching to an errored pane now keeps the error and its note
// instead of clearing both to idle.
// 12: agent sessions (Agent on PayloadSession and SessionInfo) — a session can
// name the agent kind it hosts, which the daemon starts in its first pane.
// 13: agent lifecycle — AgentStarted on SessionInfo, LastOutput on TerminalInfo,
// and MsgAck — so the Agents view can report an agent that exited or has gone
// quiet, and acknowledge one without attaching to it.
// 14: MsgTerminalTail — the last lines a pane printed, so a finished agent can
// say what it did without being attached to.
const ProtocolVersion = 14
