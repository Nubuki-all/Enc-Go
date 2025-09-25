package db

import (
    "database/sql/driver"
    "encoding/json"
    "fmt"
)

type Int64Slice []int64

// Value implements driver.Valuer: stores JSON string/bytes in DB.
func (s Int64Slice) Value() (driver.Value, error) {
    if s == nil {
        return nil, nil
    }
    b, err := json.Marshal([]int64(s))
    return string(b), err
}

// Scan implements sql.Scanner: reads DB value -> Go type.
func (s *Int64Slice) Scan(src any) error {
    if src == nil {
        *s = nil
        return nil
    }
    var data []byte
    switch v := src.(type) {
    case []byte:
        data = v
    case string:
        data = []byte(v)
    default:
        return fmt.Errorf("unsupported data type: %T", src)
    }
    var tmp []int64
    if err := json.Unmarshal(data, &tmp); err != nil {
        return err
    }
    *s = Int64Slice(tmp)
    return nil
}
