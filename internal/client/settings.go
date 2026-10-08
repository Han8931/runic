package client

import (
	"github.com/han/runic/internal/protocol"
)

func SetMaxSlots(n int) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSetMaxSlots,
		Payload: protocol.PayloadSlots{Slots: n},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func GetMaxSlots() (int, error) {
	c, err := Connect()
	if err != nil {
		return 0, err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgGetMaxSlots}); err != nil {
		return 0, err
	}
	msg, err := c.Recv()
	if err != nil {
		return 0, err
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadSlots](msg)
	if pErr != nil {
		return 0, pErr
	}
	return payload.Slots, nil
}

func GetEnv(key string) (string, error) {
	c, err := Connect()
	if err != nil {
		return "", err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgGetEnv,
		Payload: protocol.PayloadEnv{Key: key},
	}); err != nil {
		return "", err
	}
	msg, err := c.Recv()
	if err != nil {
		return "", err
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadEnv](msg)
	if pErr != nil {
		return "", pErr
	}
	return payload.Value, nil
}

func SetEnv(key, value string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSetEnv,
		Payload: protocol.PayloadEnv{Key: key, Value: value},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func UnsetEnv(key string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgUnsetEnv,
		Payload: protocol.PayloadEnv{Key: key},
	}); err != nil {
		return err
	}
	return recvOK(c)
}

func GetLogdir() (string, error) {
	c, err := Connect()
	if err != nil {
		return "", err
	}
	defer c.Close()

	if err := c.Send(&protocol.Msg{Type: protocol.MsgGetLogdir}); err != nil {
		return "", err
	}
	msg, err := c.Recv()
	if err != nil {
		return "", err
	}
	payload, pErr := protocol.PayloadAs[protocol.PayloadLogdir](msg)
	if pErr != nil {
		return "", pErr
	}
	return payload.Path, nil
}

func SetLogdir(path string) error {
	c, err := Connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Send(&protocol.Msg{
		Type:    protocol.MsgSetLogdir,
		Payload: protocol.PayloadLogdir{Path: path},
	}); err != nil {
		return err
	}
	return recvOK(c)
}
