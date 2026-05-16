package models

import "encoding/json"

func (m *MaybeInaccessibleMessage) UnmarshalJSON(data []byte) error {
	var probe struct {
		Date int `json:"date"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if probe.Date == 0 {
		var inaccessible InaccessibleMessage
		if err := json.Unmarshal(data, &inaccessible); err != nil {
			return err
		}
		m.InaccessibleMessage = &inaccessible
		return nil
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	m.Message = &msg
	return nil
}
