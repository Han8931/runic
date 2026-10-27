package client

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/han/runic/internal/protocol"
)

func SessionList() ([]string, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgSessionList}); err != nil {
		return nil, err
	}
	msg, err := c.Recv()
	if err != nil {
		return nil, err
	}
	if msg.Type == protocol.MsgError {
		return nil, recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadSessionList](msg)
	if pErr != nil {
		return nil, pErr
	}
	return payload.Sessions, nil
}

func SessionCreate(name string) error {
	return SessionCreateSpec(SessionSpec{Name: name})
}

// SessionCreateWorktree creates a session, optionally in a fresh git worktree on
// its own branch.
func SessionCreateWorktree(name string, worktree bool, repo, branch string) error {
	return SessionCreateSpec(SessionSpec{
		Name: name, Worktree: worktree, Repo: repo, Branch: branch,
	})
}

// SessionSpec describes a session to create. Everything but Name is optional,
// which is why this is a struct: a plain session, a worktree session and an
// agent session differ only in which fields they fill.
type SessionSpec struct {
	Name string
	// Worktree asks for an isolated git worktree on its own branch. Repo is the
	// repository to branch from (the caller's working directory when empty);
	// Branch defaults to the session name. The worktree directory is chosen by
	// the daemon and removed when the session is deleted.
	Worktree bool
	Repo     string
	Branch   string
	// Agent names the coding agent the session hosts ("claude"), which the
	// daemon starts in its first pane. An unknown kind fails the create.
	Agent string
}

// SessionCreateSpec creates a session as described by spec.
func SessionCreateSpec(spec SessionSpec) error {
	if spec.Worktree && spec.Repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
		spec.Repo = cwd
	}
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type: protocol.MsgSessionCreate,
		Payload: protocol.PayloadSession{
			Name:     spec.Name,
			Worktree: spec.Worktree,
			Repo:     spec.Repo,
			Branch:   spec.Branch,
			Agent:    spec.Agent,
		},
	}); err != nil {
		return err
	}
	msg, err := c.Recv()
	if err != nil {
		return err
	}
	if msg.Type == protocol.MsgError {
		return recvError(msg)
	}
	return nil
}

func SessionRename(oldName, newName string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSessionRename,
		Payload: protocol.PayloadSessionRename{OldName: oldName, NewName: newName},
	}); err != nil {
		return err
	}
	msg, err := c.Recv()
	if err != nil {
		return err
	}
	if msg.Type == protocol.MsgError {
		return recvError(msg)
	}
	return nil
}

// ErrDirtyWorktree is the sentinel for "that session delete would have thrown
// away uncommitted work". Callers that can ask the user test for it with
// errors.Is and retry via SessionDeleteDiscard; everything else prints it.
var ErrDirtyWorktree = errors.New("worktree has uncommitted work")

// DirtyWorktreeError adds the daemon's description of that work — how many
// changes, where, and the first few paths — so a caller can name the loss in
// its prompt instead of asking "are you sure?" about nothing in particular.
type DirtyWorktreeError struct{ Detail string }

func (e *DirtyWorktreeError) Error() string { return ErrDirtyWorktree.Error() + ": " + e.Detail }
func (e *DirtyWorktreeError) Unwrap() error { return ErrDirtyWorktree }

func SessionDelete(name string) error {
	return SessionDeleteDiscard(name, false)
}

// SessionDeleteDiscard deletes a session. With discard set, uncommitted work in
// the session's worktree is thrown away; without it, such a session is left
// untouched and the error wraps ErrDirtyWorktree.
func SessionDeleteDiscard(name string, discard bool) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSessionDelete,
		Payload: protocol.PayloadSession{Name: name, Discard: discard},
	}); err != nil {
		return err
	}
	msg, err := c.Recv()
	if err != nil {
		return err
	}
	if msg.Type == protocol.MsgError {
		err := recvError(msg)
		if detail, ok := strings.CutPrefix(err.Error(), protocol.DirtyWorktreePrefix); ok {
			return &DirtyWorktreeError{Detail: detail}
		}
		return err
	}
	return nil
}

func SessionMove(session, group string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSessionMove,
		Payload: protocol.PayloadSessionMove{Session: session, Group: group},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func GroupList() ([]string, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgGroupList}); err != nil {
		return nil, err
	}
	msg, err := c.Recv()
	if err != nil {
		return nil, err
	}
	if msg.Type == protocol.MsgError {
		return nil, recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadGroupList](msg)
	if pErr != nil {
		return nil, pErr
	}
	return payload.Groups, nil
}

func GroupCreate(name string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGroupCreate,
		Payload: protocol.PayloadSession{Name: name},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func GroupRename(oldName, newName string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGroupRename,
		Payload: protocol.PayloadSessionRename{OldName: oldName, NewName: newName},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func GroupDelete(name string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGroupDelete,
		Payload: protocol.PayloadSession{Name: name},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func ListJobsInSession(session string) (*ListResult, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgListSession,
		Payload: protocol.PayloadSession{Name: session},
	}); err != nil {
		return nil, err
	}

	result := &ListResult{}
	for {
		msg, err := c.Recv()
		if err != nil {
			return nil, err
		}
		switch msg.Type {
		case protocol.MsgListLine:
			payload, pErr := protocol.PayloadAs[protocol.PayloadListLine](msg)
			if pErr != nil {
				return nil, pErr
			}
			result.Jobs = append(result.Jobs, payload.Job)
		case protocol.MsgListEnd:
			payload, pErr := protocol.PayloadAs[protocol.PayloadSlots](msg)
			if pErr != nil {
				return nil, pErr
			}
			result.MaxSlots = payload.Slots
			return result, nil
		case protocol.MsgError:
			return nil, recvError(msg)
		default:
			return nil, fmt.Errorf("unexpected message: %v", msg.Type)
		}
	}
}

func ClearFinishedInSession(session string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgClearFinishedSession,
		Payload: protocol.PayloadSession{Name: session},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func TreeData() (*protocol.PayloadTreeData, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgTreeList}); err != nil {
		return nil, err
	}
	msg, err := c.Recv()
	if err != nil {
		return nil, err
	}
	if msg.Type == protocol.MsgError {
		return nil, recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadTreeData](msg)
	if pErr != nil {
		return nil, pErr
	}
	return &payload, nil
}

// RequestJobsView asks the daemon to flag that a running interactive TUI should
// open its job-management view on its next tree poll.
func RequestJobsView() error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgRequestJobsView}); err != nil {
		return err
	}
	msg, err := c.Recv()
	if err != nil {
		return err
	}
	if msg.Type == protocol.MsgError {
		return recvError(msg)
	}
	return nil
}
