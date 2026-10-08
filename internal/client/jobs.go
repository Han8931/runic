package client

import (
	"fmt"

	"github.com/han/runic/internal/protocol"
)

type ListResult struct {
	Jobs     []protocol.JobInfo
	MaxSlots int
}

func ListJobs() (*ListResult, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgList}); err != nil {
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

func ClearFinished() error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{Type: protocol.MsgClearFinished}); err != nil {
		return err
	}
	return recvOK(c)
}

func RemoveJob(id int) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgRemoveJob,
		Payload: protocol.PayloadJobID{JobID: id},
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

// BatchResult reports how a multi-job action fared: how many the daemon
// accepted, and the first refusal (all refusals share a cause in practice —
// "cannot remove job" for the running ones, say).
type BatchResult struct {
	OK      int
	Failed  int
	FirstID int   // the first id the daemon refused
	Err     error // why it refused
}

// batchByID issues one request per id over a *single* connection. Fanning the
// same work out over one connection each trips the daemon's max_conn cap (10
// by default), which silently dropped most of a large selection.
func batchByID(msgType protocol.MsgType, ids []int) (BatchResult, error) {
	var res BatchResult
	if len(ids) == 0 {
		return res, nil
	}
	c, err := Connect()
	if err != nil {
		return res, err
	}
	defer c.Close()

	for _, id := range ids {
		if err := c.Send(&protocol.Msg{
			Type:    msgType,
			Payload: protocol.PayloadJobID{JobID: id},
		}); err != nil {
			return res, err
		}
		msg, err := c.Recv()
		if err != nil {
			return res, err
		}
		if msg.Type == protocol.MsgError {
			res.Failed++
			if res.Err == nil {
				res.FirstID, res.Err = id, recvError(msg)
			}
			continue
		}
		res.OK++
	}
	return res, nil
}

// RemoveJobs removes every id over one connection. A per-job refusal (a
// running job, say) is counted, not fatal; the error return is reserved for a
// connection-level failure that aborted the batch.
func RemoveJobs(ids []int) (BatchResult, error) {
	return batchByID(protocol.MsgRemoveJob, ids)
}

// KillJobs kills every id over one connection.
func KillJobs(ids []int) (BatchResult, error) {
	return batchByID(protocol.MsgKillJob, ids)
}

// MakeUrgentJobs moves every id to the front of the queue over one connection.
func MakeUrgentJobs(ids []int) (BatchResult, error) {
	return batchByID(protocol.MsgUrgent, ids)
}

// RerunJobs re-enqueues every id over one connection.
func RerunJobs(ids []int) (BatchResult, error) {
	return batchByID(protocol.MsgRerun, ids)
}

// Rerun re-enqueues a copy of an existing job and returns the new job ID.
func Rerun(id int) (int, error) {
	c, err := Connect()
	if err != nil {
		return -1, err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgRerun,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return -1, err
	}
	msg, err := c.Recv()
	if err != nil {
		return -1, err
	}
	if msg.Type == protocol.MsgError {
		return -1, recvError(msg)
	}
	payload, perr := protocol.PayloadAs[protocol.PayloadJobID](msg)
	if perr != nil {
		return -1, perr
	}
	return payload.JobID, nil
}

func GetState(id int) (protocol.JobState, error) {
	c, err := Connect()
	if err != nil {
		return 0, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGetState,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return 0, err
	}
	msg, err := c.Recv()
	if err != nil {
		return 0, err
	}
	if msg.Type == protocol.MsgError {
		return 0, recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadState](msg)
	if pErr != nil {
		return 0, pErr
	}
	return payload.State, nil
}

func GetInfo(id int) (*protocol.JobInfo, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgInfo,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return nil, err
	}
	msg, err := c.Recv()
	if err != nil {
		return nil, err
	}
	if msg.Type == protocol.MsgError {
		return nil, recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadInfo](msg)
	if pErr != nil {
		return nil, pErr
	}
	return &payload.Job, nil
}

func GetPID(id int) (int, error) {
	info, err := GetInfo(id)
	if err != nil {
		return 0, err
	}
	return info.PID, nil
}

func MakeUrgent(id int) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgUrgent,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func SwapJobs(id1, id2 int) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSwapJobs,
		Payload: protocol.PayloadSwap{ID1: id1, ID2: id2},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func WaitJob(id int) (*protocol.Result, error) {
	c, err := Connect()
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgWaitJob,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return nil, err
	}
	msg, err := c.Recv()
	if err != nil {
		return nil, err
	}
	if msg.Type == protocol.MsgError {
		return nil, recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadResult](msg)
	if pErr != nil {
		return nil, pErr
	}
	return &payload.Result, nil
}

func KillJob(id int) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgKillJob,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func KillAllJobs() error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{Type: protocol.MsgKillAll}); err != nil {
		return err
	}
	return recvOK(c)
}

func CountRunning() (int, error) {
	c, err := Connect()
	if err != nil {
		return 0, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgCountRunning}); err != nil {
		return 0, err
	}
	msg, err := c.Recv()
	if err != nil {
		return 0, err
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadCount](msg)
	if pErr != nil {
		return 0, pErr
	}
	return payload.Count, nil
}

func GetLabel(id int) (string, error) {
	c, err := Connect()
	if err != nil {
		return "", err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGetLabel,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return "", err
	}
	msg, err := c.Recv()
	if err != nil {
		return "", err
	}
	if msg.Type == protocol.MsgError {
		return "", recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadLabel](msg)
	if pErr != nil {
		return "", pErr
	}
	return payload.Label, nil
}

// SetJobLabel renames an existing job (sets its label).
func SetJobLabel(id int, label string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSetJobLabel,
		Payload: protocol.PayloadSetLabel{JobID: id, Label: label},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

// SetJobTimeout changes a job's wall-clock timeout (0 clears it); it takes
// effect when the job (re)starts.
func SetJobTimeout(id int, timeoutMS int64) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSetJobTimeout,
		Payload: protocol.PayloadSetTimeout{JobID: id, TimeoutMS: timeoutMS},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func LastID() (int, error) {
	c, err := Connect()
	if err != nil {
		return 0, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgLastID}); err != nil {
		return 0, err
	}
	msg, err := c.Recv()
	if err != nil {
		return 0, err
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadJobID](msg)
	if pErr != nil {
		return 0, pErr
	}
	return payload.JobID, nil
}

func GetCmd(id int) (string, error) {
	c, err := Connect()
	if err != nil {
		return "", err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGetCmd,
		Payload: protocol.PayloadJobID{JobID: id},
	}); err != nil {
		return "", err
	}
	msg, err := c.Recv()
	if err != nil {
		return "", err
	}
	if msg.Type == protocol.MsgError {
		return "", recvError(msg)
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadCmd](msg)
	if pErr != nil {
		return "", pErr
	}
	return payload.Cmd, nil
}
