package protocol

// ProtocolVersion gates client/daemon compatibility: a client refuses to talk
// to a daemon speaking a different version rather than guess at the wire.
// Bump it whenever a message type or payload shape changes.
//
// 10: MsgMark + per-pane attention state (Attention on SessionInfo/TerminalInfo).
const ProtocolVersion = 10
