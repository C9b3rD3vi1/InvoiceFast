package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Money represents monetary values in cents to avoid floating-point precision loss.
// Stores as bigint in PostgreSQL, serializes as float in JSON.
type Money int64

func (m Money) String() string {
	return strconv.FormatInt(int64(m), 10)
}

func (m Money) Float64() float64 {
	return float64(m) / 100
}

func (m Money) Add(other Money) Money {
	return m + other
}

func (m Money) Sub(other Money) Money {
	return m - other
}

func (m Money) Subtract(other Money) Money {
	return m - other
}

func (m Money) Mul(factor float64) Money {
	return Money(math.Round(float64(m) * factor))
}

func (m Money) Multiply(factor float64) Money {
	return Money(math.Round(float64(m) * factor))
}

func (m Money) Div(divisor float64) Money {
	if divisor == 0 {
		return 0
	}
	return Money(math.Round(float64(m) / divisor))
}

func (m Money) Divide(divisor float64) Money {
	if divisor == 0 {
		return 0
	}
	return Money(math.Round(float64(m) / divisor))
}

func (m Money) IsZero() bool {
	return m == 0
}

func (m Money) IsPositive() bool {
	return m > 0
}

func (m Money) Gt(other Money) bool {
	return m > other
}

func (m Money) GreaterThan(other Money) bool {
	return m > other
}

func (m Money) Lt(other Money) bool {
	return m < other
}

func (m Money) LessThan(other Money) bool {
	return m < other
}

func (m Money) Equals(other Money) bool {
	return m == other
}

func ToCents(amount float64) Money {
	return Money(math.Round(amount * 100))
}

func FromCents(cents int64) Money {
	return Money(cents)
}

// SQL / GORM interfaces

func (m *Money) Scan(src interface{}) error {
	if src == nil {
		*m = 0
		return nil
	}
	switch v := src.(type) {
	case int64:
		*m = Money(v)
	case float64:
		*m = ToCents(v)
	case []byte:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return err
		}
		*m = Money(parsed)
	default:
		return fmt.Errorf("unsupported Money scan type: %T", src)
	}
	return nil
}

func (m Money) Value() (driver.Value, error) {
	return int64(m), nil
}

func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Float64())
}

func (m *Money) UnmarshalJSON(data []byte) error {
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	*m = ToCents(f)
	return nil
}

func (m Money) GormDataType() string {
	return "bigint"
}

func (m Money) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	if db.Dialector.Name() == "postgres" {
		return "bigint"
	}
	return "integer"
}

func RoundMoney(v float64) Money {
	return ToCents(v)
}
