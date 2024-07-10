package wsdelivery

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hpcloud/tail"
)

func tailLog(ctx context.Context, path string) <-chan Message[string] {
	ch := make(chan Message[string])
	go func() {
		f, err := tail.TailFile(path, tail.Config{Follow: true, Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd}})
		if err != nil {
			ch <- Message[string]{Err: fmt.Errorf("tail file: %w", err)}
			return
		}
		defer f.Cleanup()
		for {
			select {
			case <-ctx.Done():
				return

			case v := <-f.Lines:
				if v == nil {
					ch <- Message[string]{Err: fmt.Errorf("tail: got nil line. waiting 1 second")}
					select {
					case <-ctx.Done():
					case <-time.After(time.Second):
					}
					continue
				}
				if v.Err != nil {
					ch <- Message[string]{Err: fmt.Errorf("read lines: %w", err)}
					continue
				}
				ch <- Message[string]{Body: v.Text}
			}
		}
	}()
	return ch
}
