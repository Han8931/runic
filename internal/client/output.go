package client

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/han/runic/internal/protocol"
)

func GetOutput(id int) (string, error) {
	c, err := Connect()
	if err != nil {
		return "", err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgAskOutput,
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
	payload, pErr := protocol.PayloadAs[protocol.PayloadOutput](msg)
	if pErr != nil {
		return "", pErr
	}
	return payload.Filename, nil
}

func CatOutput(id int) error {
	filename, err := GetOutput(id)
	if err != nil {
		return err
	}
	if filename == "" {
		return fmt.Errorf("no output file for job %d", id)
	}

	for {
		if _, err := os.Stat(filename); err == nil {
			break
		}
		state, err := GetState(id)
		if err != nil {
			return err
		}
		if state == protocol.StateFinished || state == protocol.StateSkipped {
			if _, statErr := os.Stat(filename); statErr != nil {
				return fmt.Errorf("job finished but no output file")
			}
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4096)
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			os.Stdout.Write(buf[:n])
		}
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if readErr == io.EOF {
			state, stateErr := GetState(id)
			if stateErr != nil {
				return reportExitStatus(id)
			}
			if state == protocol.StateFinished || state == protocol.StateSkipped {
				for {
					n, _ := f.Read(buf)
					if n == 0 {
						break
					}
					os.Stdout.Write(buf[:n])
				}
				return reportExitStatus(id)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func reportExitStatus(id int) error {
	info, err := GetInfo(id)
	if err != nil {
		return nil
	}
	if info.State == protocol.StateSkipped {
		fmt.Fprintf(os.Stderr, "ru: job %d was skipped (dependency failed)\n", id)
		return nil
	}
	if info.State != protocol.StateFinished {
		return nil
	}
	if info.Result.Canceled {
		fmt.Fprintf(os.Stderr, "ru: job %d was canceled\n", id)
		return nil
	}
	if info.Result.DiedBySignal {
		fmt.Fprintf(os.Stderr, "ru: job %d killed by signal %d\n", id, info.Result.Signal)
		return nil
	}
	if info.Result.ExitCode != 0 {
		fmt.Fprintf(os.Stderr, "ru: job %d exited with code %d\n", id, info.Result.ExitCode)
	}
	return nil
}

func ShowOutputFile(id int) error {
	filename, err := GetOutput(id)
	if err != nil {
		return err
	}
	fmt.Println(filename)
	return nil
}
