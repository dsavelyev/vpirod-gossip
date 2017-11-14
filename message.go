package gossip

import (
	"encoding/json"
	"fmt"
	"strings"
)

type messageTypeEnum int

const (
	MULTICAST messageTypeEnum = iota
	NOTIFICATION
)

type message struct {
	ID     int             `json:"id"`
	Type   messageTypeEnum `json:"type"`
	Sender int             `json:"sender"`
	Origin int             `json:"origin"`
	Data   string          `json:"data"`
}

// Implementation of json.(Un)marshaler for the "enum"
func (ty *messageTypeEnum) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	s = strings.ToLower(s)
	switch s {
	case "multicast":
		*ty = MULTICAST
	case "notification":
		*ty = NOTIFICATION
	default:
		return fmt.Errorf("invalid msg type %s", s)
	}

	return nil
}

func (ty messageTypeEnum) MarshalJSON() ([]byte, error) {
	var s string

	switch ty {
	case MULTICAST:
		s = "multicast"
	case NOTIFICATION:
		s = "notification"
	default:
		return nil, fmt.Errorf("invalid msg type %d", ty)
	}

	return json.Marshal(s)
}

// Helper functions
func toJSON(msg *message) []byte {
	sl, err := json.Marshal(*msg)
	if err != nil {
		panic(err)
	}
	return sl
}

func fromJSON(obj []byte) *message {
	var msg message
	if err := json.Unmarshal(obj, &msg); err != nil {
		panic(err)
	}
	return &msg
}
