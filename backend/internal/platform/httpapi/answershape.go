package httpapi

import (
	"encoding/json"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
)

type answerHeaders struct {
	replayed   bool
	retryAfter *int
}

type answerShape func([]byte, answerHeaders) (any, error)

func shapeOf[T any](spell func(T, answerHeaders) any) answerShape {
	return func(stored []byte, headers answerHeaders) (any, error) {
		var body T
		if err := json.Unmarshal(stored, &body); err != nil {
			return nil, err
		}
		return spell(body, headers), nil
	}
}

func (headers answerHeaders) retry(code servedapi.ErrorCode) *int {
	if headers.retryAfter != nil {
		return headers.retryAfter
	}
	return retryAfterOf(code)
}
