package wsdelivery

type Message[T any] struct {
	Body T
	Err  error
}
