# rhetoric-go

A Go client for the Rhetoric e-filing API.

## Install

```bash
go get github.com/rhetoriclabs/rhetoric-go
```

## Usage

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	rhetoric "github.com/rhetoriclabs/rhetoric-go"
)

func main() {
	ctx := context.Background()

	client := rhetoric.New(
		"https://api.rhetoric.law",
		rhetoric.WithToken(os.Getenv("RHETORIC_API_KEY")),
	)

	created, err := client.CreateFiler(ctx, rhetoric.CreateFilerInput{})
	if err != nil {
		handle(err)
		return
	}

	session, err := client.CreateFilingSession(ctx, rhetoric.CreateFilingSessionInput{
		FilerID: created.Filer.ID,
	})
	if err != nil {
		handle(err)
		return
	}
	fmt.Println(session.SessionToken)

	list, err := client.ListFilingSessions(ctx, rhetoric.ListFilingSessionsInput{
		FilerID: rhetoric.Ptr(created.Filer.ID),
		Limit:   rhetoric.Ptr(uint8(20)),
	})
	if err != nil {
		handle(err)
		return
	}
	fmt.Println(list.Sessions)
}

func handle(err error) {
	var transportErr *rhetoric.OperationTransportError
	if errors.As(err, &transportErr) {
		fmt.Println("transport error:", transportErr.Err)
		return
	}

	// Operations with application errors return OperationAppError[E], where E is
	// the operation's error type, e.g. rhetoric.ListFilersError.
	var appErr *rhetoric.OperationAppError[rhetoric.ListFilersError]
	if errors.As(err, &appErr) {
		fmt.Println("application error:", appErr.Data)
	}
}
