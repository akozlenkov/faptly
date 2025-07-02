package manager

import "time"

type Date struct {
	time.Time
}

func (d Date) MarshalControl() (string, error) {
	return d.Format(time.RFC1123), nil
}

func (d *Date) UnmarshalControl(data string) error {
	t, err := time.Parse(time.RFC1123, data)
	if err != nil {
		return err
	}
	d.Time = t.UTC()
	return nil
}
