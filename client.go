// Package rhetoric is a client for the Rhetoric service.
package rhetoric

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Client is a client for the Rhetoric service.
type Client struct {
	url        string
	token      string
	httpClient *http.Client
}

// Option configures a [Client].
type Option func(*Client)

// WithToken sets the auth token for the service.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithHTTPClient sets the HTTP client used to send requests.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) { c.httpClient = httpClient }
}

// New constructs a [Client]. url is the base URL for the service.
func New(url string, opts ...Option) *Client {
	c := &Client{
		url:        url,
		httpClient: http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Creates a new filer.
//
// A filer represents an entity that files documents through Rhetoric, such as
// an individual or an organization.
func (c *Client) CreateFiler(ctx context.Context, input CreateFilerInput) (CreateFilerOutput, error) {
	var output CreateFilerOutput
	if err := call[struct{}](ctx, c, "create-filer", input, &output, nil); err != nil {
		return CreateFilerOutput{}, err
	}
	return output, nil
}

// Gets a filer.
func (c *Client) GetFiler(ctx context.Context, input GetFilerInput) (GetFilerOutput, error) {
	var output GetFilerOutput
	if err := call(ctx, c, "get-filer", input, &output, decodeGetFilerError); err != nil {
		return GetFilerOutput{}, err
	}
	return output, nil
}

// Returns a list of filers.
//
// The results are returned in reverse chronological order by creation date.
func (c *Client) ListFilers(ctx context.Context, input ListFilersInput) (ListFilersOutput, error) {
	var output ListFilersOutput
	if err := call(ctx, c, "list-filers", input, &output, decodeListFilersError); err != nil {
		return ListFilersOutput{}, err
	}
	return output, nil
}

// Creates a filing session.
func (c *Client) CreateFilingSession(ctx context.Context, input CreateFilingSessionInput) (CreateFilingSessionOutput, error) {
	var output CreateFilingSessionOutput
	if err := call[struct{}](ctx, c, "create-filing-session", input, &output, nil); err != nil {
		return CreateFilingSessionOutput{}, err
	}
	return output, nil
}

// Returns a filing session.
func (c *Client) GetFilingSession(ctx context.Context, input GetFilingSessionInput) (GetFilingSessionOutput, error) {
	var output GetFilingSessionOutput
	if err := call(ctx, c, "get-filing-session", input, &output, decodeGetFilingSessionError); err != nil {
		return GetFilingSessionOutput{}, err
	}
	return output, nil
}

// Returns a list of filing sessions.
func (c *Client) ListFilingSessions(ctx context.Context, input ListFilingSessionsInput) (ListFilingSessionsOutput, error) {
	var output ListFilingSessionsOutput
	if err := call(ctx, c, "list-filing-sessions", input, &output, decodeListFilingSessionsError); err != nil {
		return ListFilingSessionsOutput{}, err
	}
	return output, nil
}

// Returns the current form of a filing session.
//
// Authenticated with a session token instead of an API key.
func (c *Client) ResumeFilingSession(ctx context.Context, input ResumeFilingSessionInput) (ResumeFilingSessionOutput, error) {
	var output ResumeFilingSessionOutput
	if err := call(ctx, c, "resume-filing-session", input, &output, decodeResumeFilingSessionError); err != nil {
		return ResumeFilingSessionOutput{}, err
	}
	return output, nil
}

// Applies an event to a filing session and returns the resulting form.
//
// Authenticated with the session token instead of an API key.
func (c *Client) ApplyFilingSessionEvent(ctx context.Context, input ApplyFilingSessionEventInput) (ApplyFilingSessionEventOutput, error) {
	var output ApplyFilingSessionEventOutput
	if err := call(ctx, c, "apply-filing-session-event", input, &output, decodeApplyFilingSessionEventError); err != nil {
		return ApplyFilingSessionEventOutput{}, err
	}
	return output, nil
}

// OperationTransportError is returned when an operation fails during transport
// (e.g. a network failure, an internal server error, invalid authentication, or
// a response that does not match the expected shape).
type OperationTransportError struct {
	// Err is the underlying cause.
	Err error
}

func (e *OperationTransportError) Error() string {
	return fmt.Sprintf("transport error: %v", e.Err)
}

func (e *OperationTransportError) Unwrap() error { return e.Err }

// OperationAppError is returned when the server responds with a well-formed
// application error.
type OperationAppError[E any] struct {
	// Data is the application error returned by the server.
	Data E
}

func (e *OperationAppError[E]) Error() string {
	return fmt.Sprintf("application error: %v", e.Data)
}

// call sends a request to the server and decodes the response. input is
// serialized to JSON (nil sends no body). On success the response is decoded
// into output (nil discards it). If the server returns an application error, it
// is decoded by decodeErr (nil if the operation has no error type) and returned
// as an [*OperationAppError]. Any other failure is returned as an
// [*OperationTransportError].
//
// This is a function rather than a method because Go methods cannot have type
// parameters.
func call[E any](
	ctx context.Context,
	c *Client,
	method string,
	input any,
	output any,
	decodeErr func([]byte) (E, error),
) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return &OperationTransportError{err}
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/"+method, body)
	if err != nil {
		return &OperationTransportError{err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return &OperationTransportError{err}
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return &OperationTransportError{fmt.Errorf("unexpected status code: %d", res.StatusCode)}
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return &OperationTransportError{err}
	}

	// the result is a sibling of the fields of the output or error, which are
	// decoded without it
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return &OperationTransportError{err}
	}
	isErr := string(fields["result"]) == `"err"`
	delete(fields, "result")
	if data, err = json.Marshal(fields); err != nil {
		return &OperationTransportError{err}
	}

	if isErr {
		if decodeErr == nil {
			return &OperationTransportError{errors.New("received an application error with no error type")}
		}
		appErr, err := decodeErr(data)
		if err != nil {
			return &OperationTransportError{err}
		}
		return &OperationAppError[E]{appErr}
	}

	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return &OperationTransportError{err}
		}
	}
	return nil
}

// requireFields checks that data is a JSON object containing every key in
// required with a non-null value, and every key in nullable with any value
// (including null). Keys that are not listed are ignored.
func requireFields(data []byte, required []string, nullable []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range required {
		raw, ok := fields[name]
		if !ok {
			return fmt.Errorf("missing required field %q", name)
		}
		if string(raw) == "null" {
			return fmt.Errorf("required field %q is null", name)
		}
	}
	for _, name := range nullable {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("missing required field %q", name)
		}
	}
	return nil
}

// Ptr returns a pointer to v. It is a convenience for populating optional
// fields, which are pointers, from literals and other values that cannot be
// addressed directly:
//
//	UpdateTodoInput{ID: id, Done: Ptr(true)}
func Ptr[T any](v T) *T {
	return &v
}

// decodeOpt decodes raw with decode, or returns the zero value if raw is absent
// or null.
func decodeOpt[T any](raw json.RawMessage, decode func([]byte) (T, error)) (T, error) {
	var zero T
	if len(raw) == 0 || string(raw) == "null" {
		return zero, nil
	}
	return decode(raw)
}

// decodeList decodes a JSON array, decoding each element with decode. It
// returns nil if raw is absent or null.
func decodeList[T any](raw json.RawMessage, decode func([]byte) (T, error)) ([]T, error) {
	var raws []json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &raws); err != nil {
			return nil, err
		}
	}
	if raws == nil {
		return nil, nil
	}
	out := make([]T, len(raws))
	for i, r := range raws {
		v, err := decode(r)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out[i] = v
	}
	return out, nil
}

// decodeMap decodes a JSON object, decoding each value with decode. It returns
// nil if raw is absent or null.
func decodeMap[K comparable, T any](raw json.RawMessage, decode func([]byte) (T, error)) (map[K]T, error) {
	var raws map[K]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &raws); err != nil {
			return nil, err
		}
	}
	if raws == nil {
		return nil, nil
	}
	out := make(map[K]T, len(raws))
	for k, r := range raws {
		v, err := decode(r)
		if err != nil {
			return nil, fmt.Errorf("key %v: %w", k, err)
		}
		out[k] = v
	}
	return out, nil
}

// marshalTagged marshals v, which must encode as a JSON object, with the field
// tagField set to tag.
func marshalTagged(tagField string, tag string, v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	if fields[tagField], err = json.Marshal(tag); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

// A filer.
type Filer struct {
	// The filer ID.
	ID string `json:"id"`
	// The time the filer was created, as an ISO 8601 timestamp.
	CreatedAt string `json:"created_at"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Filer) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"id", "created_at"}, nil); err != nil {
		return err
	}
	type plain Filer
	return json.Unmarshal(data, (*plain)(v))
}

// The input for the create filer operation.
type CreateFilerInput struct{}

// The output for the create filer operation.
type CreateFilerOutput struct {
	// The created filer.
	Filer Filer `json:"filer"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *CreateFilerOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"filer"}, nil); err != nil {
		return err
	}
	type plain CreateFilerOutput
	return json.Unmarshal(data, (*plain)(v))
}

// The input for the get filer operation.
type GetFilerInput struct {
	// The filer ID.
	ID string `json:"id"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *GetFilerInput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"id"}, nil); err != nil {
		return err
	}
	type plain GetFilerInput
	return json.Unmarshal(data, (*plain)(v))
}

// The output for the get filer operation.
type GetFilerOutput struct {
	// The identified filer.
	Filer Filer `json:"filer"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *GetFilerOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"filer"}, nil); err != nil {
		return err
	}
	type plain GetFilerOutput
	return json.Unmarshal(data, (*plain)(v))
}

// The input for the list filers operation.
type ListFilersInput struct {
	// The number of results to return.
	//
	// The limit must be between 1 and 100 (default: 10).
	Limit *uint8 `json:"limit,omitempty"`
	// The token for the next page of results.
	//
	// If none, the operation returns the first page of results.
	NextToken *string `json:"next_token,omitempty"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ListFilersInput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, nil, nil); err != nil {
		return err
	}
	type plain ListFilersInput
	return json.Unmarshal(data, (*plain)(v))
}

// The output for the list filers operation.
type ListFilersOutput struct {
	// The filers.
	Filers []Filer `json:"filers"`
	// The total available results.
	TotalCount uint64 `json:"total_count"`
	// The token for the next page of results.
	//
	// If none, there are no more results.
	NextToken *string `json:"next_token,omitempty"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ListFilersOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"filers", "total_count"}, nil); err != nil {
		return err
	}
	type plain ListFilersOutput
	return json.Unmarshal(data, (*plain)(v))
}

// A filing session.
type FilingSession struct {
	// The session ID.
	ID string `json:"id"`
	// The time the session was created, as an ISO 8601 timestamp.
	CreatedAt string `json:"created_at"`
	// The filer ID.
	FilerID string `json:"filer_id"`
	// The status of the session.
	Status FilingSessionStatus `json:"status"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *FilingSession) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"id", "created_at", "filer_id", "status"}, nil); err != nil {
		return err
	}
	type plain FilingSession
	var aux struct {
		*plain
		Status json.RawMessage `json:"status"`
	}
	aux.plain = (*plain)(v)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if v.Status, err = decodeFilingSessionStatus(aux.Status); err != nil {
		return fmt.Errorf("field %q: %w", "status", err)
	}
	return nil
}

// The input for the create filing session operation.
type CreateFilingSessionInput struct {
	// The ID of the filer operating the filing session.
	FilerID string `json:"filer_id"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *CreateFilingSessionInput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"filer_id"}, nil); err != nil {
		return err
	}
	type plain CreateFilingSessionInput
	return json.Unmarshal(data, (*plain)(v))
}

// The output for the create filing session operation.
type CreateFilingSessionOutput struct {
	// The created session.
	Session FilingSession `json:"session"`
	// The bearer token used to authenticate session requests.
	//
	// The Element component embedded in the browser will use this to drive the
	// session. This token is only returned once and cannot be recovered; keep it
	// somewhere safe.
	SessionToken string `json:"session_token"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *CreateFilingSessionOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"session", "session_token"}, nil); err != nil {
		return err
	}
	type plain CreateFilingSessionOutput
	return json.Unmarshal(data, (*plain)(v))
}

// The input for the get filing session operation.
type GetFilingSessionInput struct {
	// The ID of the session.
	SessionID string `json:"session_id"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *GetFilingSessionInput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"session_id"}, nil); err != nil {
		return err
	}
	type plain GetFilingSessionInput
	return json.Unmarshal(data, (*plain)(v))
}

// The output for the get filing session operation.
type GetFilingSessionOutput struct {
	// The session.
	Session FilingSession `json:"session"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *GetFilingSessionOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"session"}, nil); err != nil {
		return err
	}
	type plain GetFilingSessionOutput
	return json.Unmarshal(data, (*plain)(v))
}

// The input for the list filing sessions operation.
type ListFilingSessionsInput struct {
	// Only return sessions associated with this filer.
	FilerID *string `json:"filer_id,omitempty"`
	// The number of results to return.
	//
	// The limit must be between 1 and 100 (default: 10).
	Limit *uint8 `json:"limit,omitempty"`
	// The token for the next page of results.
	//
	// If none, the operation returns the first page of results.
	NextToken *string `json:"next_token,omitempty"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ListFilingSessionsInput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, nil, nil); err != nil {
		return err
	}
	type plain ListFilingSessionsInput
	return json.Unmarshal(data, (*plain)(v))
}

// The output for the list filing sessions operation.
type ListFilingSessionsOutput struct {
	// The filing sessions.
	Sessions []FilingSession `json:"sessions"`
	// The total available results.
	TotalCount uint64 `json:"total_count"`
	// The token for the next page of results.
	//
	// If none, there are no more results.
	NextToken *string `json:"next_token,omitempty"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ListFilingSessionsOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"sessions", "total_count"}, nil); err != nil {
		return err
	}
	type plain ListFilingSessionsOutput
	return json.Unmarshal(data, (*plain)(v))
}

type ResumeFilingSessionInput struct{}

type ResumeFilingSessionOutput struct {
	// The form to render.
	Form Form `json:"form"`
	// The current state version of the session.
	//
	// Echo this back when sending events to the session to ensure proper
	// concurrency handling.
	StateVersion int64 `json:"state_version"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ResumeFilingSessionOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"form", "state_version"}, nil); err != nil {
		return err
	}
	type plain ResumeFilingSessionOutput
	return json.Unmarshal(data, (*plain)(v))
}

type ApplyFilingSessionEventInput struct {
	// The state version of the session that this event applies to.
	StateVersion int64 `json:"state_version"`
	// The event to process.
	Event Event `json:"event"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ApplyFilingSessionEventInput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"state_version", "event"}, nil); err != nil {
		return err
	}
	type plain ApplyFilingSessionEventInput
	var aux struct {
		*plain
		Event json.RawMessage `json:"event"`
	}
	aux.plain = (*plain)(v)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if v.Event, err = decodeEvent(aux.Event); err != nil {
		return fmt.Errorf("field %q: %w", "event", err)
	}
	return nil
}

type ApplyFilingSessionEventOutput struct {
	// The form to render.
	Form Form `json:"form"`
	// The current state version of the session.
	//
	// Echo this back when sending events to the session to ensure proper
	// concurrency handling.
	StateVersion int64 `json:"state_version"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ApplyFilingSessionEventOutput) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"form", "state_version"}, nil); err != nil {
		return err
	}
	type plain ApplyFilingSessionEventOutput
	return json.Unmarshal(data, (*plain)(v))
}

type Form struct {
	// The title of the form.
	Title string `json:"title"`
	// Blocks of form content.
	Blocks []Block `json:"blocks"`
	// The actions available.
	Actions []Action `json:"actions"`
	// Breadcrumb labels, one per frame on the step stack, from outermost to
	// innermost (the current step is last).
	Breadcrumbs []string `json:"breadcrumbs"`
	// Form-wide error messages that are not attributable to a single field.
	Errors []string `json:"errors"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Form) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"title", "blocks", "actions", "breadcrumbs", "errors"}, nil); err != nil {
		return err
	}
	type plain Form
	return json.Unmarshal(data, (*plain)(v))
}

// A group of items that belong together.
//
// A block says only that its items are related; it places no restriction on
// which kinds of item it holds, so input, information, and explanatory text can
// be mixed freely.
type Block struct {
	// A stable identifier for the block.
	ID string `json:"id"`
	// A heading for the block.
	Title *string `json:"title,omitempty"`
	// The content of the block.
	Items []Item `json:"items"`
	// Actions scoped to this block.
	Actions []Action `json:"actions"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Block) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"id", "items", "actions"}, nil); err != nil {
		return err
	}
	type plain Block
	var aux struct {
		*plain
		Items json.RawMessage `json:"items"`
	}
	aux.plain = (*plain)(v)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if v.Items, err = decodeList(aux.Items, decodeItem); err != nil {
		return fmt.Errorf("field %q: %w", "items", err)
	}
	return nil
}

// An input field.
type Field struct {
	// The key identifying this field.
	Key string `json:"key"`
	// The human readable label.
	Label string `json:"label"`
	// Help text shown with the field.
	Description *string `json:"description,omitempty"`
	// Whether the field must be filled before the step can advance.
	Required bool `json:"required"`
	// Whether the field can currently be interacted with.
	Disabled bool `json:"disabled"`
	// Indicates that the value is a secret, such as a password or a payment
	// detail. Renderers should mask it, for example with a password input instead
	// of a plain text one.
	Sensitive bool `json:"sensitive"`
	// The browser autofill value represented by the field. This is an HTML hint
	// that other renderers may ignore.
	Autocomplete Autocomplete `json:"autocomplete,omitempty"`
	// The virtual keyboard a browser should prefer for a text field. A hint that
	// other renderers may ignore.
	InputMode TextInputMode `json:"input_mode,omitempty"`
	// The representation of the value.
	Ty ValueType `json:"ty"`
	// The fixed set of values the field may take.
	//
	// If present, the value must be one of the options (an empty list means there
	// is nothing to choose yet). If absent, any value of the field type is
	// permitted, subject to its validations.
	//
	// At most one of `options` and `search` is present.
	Options []FieldOption `json:"options,omitempty"`
	// A set of values the field may take that is too large to send whole, and is
	// queried instead.
	//
	// If present, the value must be one of the values the search can return. Use a
	// "search" event to change the query. If absent, see `options`.
	//
	// At most one of `options` and `search` is present.
	Search *Search `json:"search,omitempty"`
	// Rules the value must satisfy.
	Validations []Validation `json:"validations"`
	// The current value, if any.
	Value json.RawMessage `json:"value,omitempty"`
	// Problems with the current value reported by the server.
	Errors []string `json:"errors"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Field) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"key", "label", "required", "disabled", "sensitive", "ty", "validations", "errors"}, nil); err != nil {
		return err
	}
	type plain Field
	var aux struct {
		*plain
		Autocomplete json.RawMessage `json:"autocomplete"`
		InputMode    json.RawMessage `json:"input_mode"`
		Ty           json.RawMessage `json:"ty"`
	}
	aux.plain = (*plain)(v)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if v.Autocomplete, err = decodeOpt(aux.Autocomplete, decodeAutocomplete); err != nil {
		return fmt.Errorf("field %q: %w", "autocomplete", err)
	}
	if v.InputMode, err = decodeOpt(aux.InputMode, decodeTextInputMode); err != nil {
		return fmt.Errorf("field %q: %w", "input_mode", err)
	}
	if v.Ty, err = decodeValueType(aux.Ty); err != nil {
		return fmt.Errorf("field %q: %w", "ty", err)
	}
	return nil
}

// A validation on an input value.
//
// Validations run after an input value has been committed and do not modify the
// input.
type Validation struct {
	// The human readable error message shown when the rule is not satisfied.
	Label string `json:"label"`
	// The rule the value must satisfy.
	Rule Rule `json:"rule"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Validation) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"label", "rule"}, nil); err != nil {
		return err
	}
	type plain Validation
	var aux struct {
		*plain
		Rule json.RawMessage `json:"rule"`
	}
	aux.plain = (*plain)(v)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	if v.Rule, err = decodeRule(aux.Rule); err != nil {
		return fmt.Errorf("field %q: %w", "rule", err)
	}
	return nil
}

// A queryable set of values for a field.
type Search struct {
	// The search query.
	Query *string `json:"query,omitempty"`
	// The values matching the query.
	Results []FieldOption `json:"results"`
	// The currently selected value, so that its label can be shown when it is not
	// among the `results`.
	Selected *FieldOption `json:"selected,omitempty"`
	// The total number of values matching the query. If it exceeds the number of
	// results count, it means the results were truncated. Absent if the server
	// cannot calculate the total.
	Total *int64 `json:"total,omitempty"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Search) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"results"}, nil); err != nil {
		return err
	}
	type plain Search
	return json.Unmarshal(data, (*plain)(v))
}

// An option for a field.
type FieldOption struct {
	// The value of this option.
	//
	// It is guaranteed to match the type of the field.
	Value json.RawMessage `json:"value"`
	// The human readable label for the option.
	Label string `json:"label"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *FieldOption) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"label"}, []string{"value"}); err != nil {
		return err
	}
	type plain FieldOption
	return json.Unmarshal(data, (*plain)(v))
}

// A labeled piece of data.
type Data struct {
	// The human readable name for the data.
	Label string `json:"label"`
	// The value of the data, formatted for display, such as `$435.00`.
	Value string `json:"value"`
	// Secondary information about the value, such as the type and page count of a
	// document or the role of a party.
	Detail *string `json:"detail,omitempty"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Data) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"label", "value"}, nil); err != nil {
		return err
	}
	type plain Data
	return json.Unmarshal(data, (*plain)(v))
}

// A paragraph of explanatory text, such as guidance or a note about what a step
// does. Use one item per paragraph.
type Text struct {
	// The text, as plain text with no formatting.
	Text string `json:"text"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Text) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"text"}, nil); err != nil {
		return err
	}
	type plain Text
	return json.Unmarshal(data, (*plain)(v))
}

// An action the user can take.
type Action struct {
	// The ID of the action.
	ActionID string `json:"action_id"`
	// The human readable label.
	Label string `json:"label"`
	// Whether this is the step's primary action.
	Primary bool `json:"primary"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *Action) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"action_id", "label", "primary"}, nil); err != nil {
		return err
	}
	type plain Action
	return json.Unmarshal(data, (*plain)(v))
}

// An error returned by the get filer operation.
//
// GetFilerError is one of [GetFilerErrorNotFound]. Use a type switch to tell
// the variants apart.
type GetFilerError interface {
	isGetFilerError()
}

// The filer was not found.
type GetFilerErrorNotFound struct{}

func (GetFilerErrorNotFound) isGetFilerError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v GetFilerErrorNotFound) MarshalJSON() ([]byte, error) {
	return marshalTagged("code", "not_found", struct{}{})
}

// decodeGetFilerError decodes a [GetFilerError] from its wire representation.
func decodeGetFilerError(data []byte) (GetFilerError, error) {
	var tag struct {
		Tag string `json:"code"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "not_found":
		return GetFilerErrorNotFound{}, nil
	}
	return nil, fmt.Errorf("unknown GetFilerError tag: %q", tag.Tag)
}

// An error returned by the list filers operation.
//
// ListFilersError is one of [ListFilersErrorInvalidLimit]. Use a type switch to
// tell the variants apart.
type ListFilersError interface {
	isListFilersError()
}

// The provided limit is out of bounds.
type ListFilersErrorInvalidLimit struct {
	// The minimum limit.
	Min uint8 `json:"min"`
	// The maximum limit.
	Max uint8 `json:"max"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ListFilersErrorInvalidLimit) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"min", "max"}, nil); err != nil {
		return err
	}
	type plain ListFilersErrorInvalidLimit
	return json.Unmarshal(data, (*plain)(v))
}

func (ListFilersErrorInvalidLimit) isListFilersError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ListFilersErrorInvalidLimit) MarshalJSON() ([]byte, error) {
	type plain ListFilersErrorInvalidLimit
	return marshalTagged("code", "invalid_limit", plain(v))
}

// decodeListFilersError decodes a [ListFilersError] from its wire representation.
func decodeListFilersError(data []byte) (ListFilersError, error) {
	var tag struct {
		Tag string `json:"code"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "invalid_limit":
		var v ListFilersErrorInvalidLimit
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("unknown ListFilersError tag: %q", tag.Tag)
}

// The status of a session.
//
// FilingSessionStatus is one of [FilingSessionStatusActive],
// [FilingSessionStatusSubmitted], [FilingSessionStatusExpired]. Use a type
// switch to tell the variants apart.
type FilingSessionStatus interface {
	isFilingSessionStatus()
}

// The session is active.
type FilingSessionStatusActive struct{}

func (FilingSessionStatusActive) isFilingSessionStatus() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v FilingSessionStatusActive) MarshalJSON() ([]byte, error) {
	return json.Marshal("active")
}

// The session has been submitted.
type FilingSessionStatusSubmitted struct{}

func (FilingSessionStatusSubmitted) isFilingSessionStatus() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v FilingSessionStatusSubmitted) MarshalJSON() ([]byte, error) {
	return json.Marshal("submitted")
}

// The session has expired.
type FilingSessionStatusExpired struct{}

func (FilingSessionStatusExpired) isFilingSessionStatus() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v FilingSessionStatusExpired) MarshalJSON() ([]byte, error) {
	return json.Marshal("expired")
}

// decodeFilingSessionStatus decodes a [FilingSessionStatus] from its wire representation.
func decodeFilingSessionStatus(data []byte) (FilingSessionStatus, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return nil, err
		}
		switch tag {
		case "active":
			return FilingSessionStatusActive{}, nil
		case "submitted":
			return FilingSessionStatusSubmitted{}, nil
		case "expired":
			return FilingSessionStatusExpired{}, nil
		}
		return nil, fmt.Errorf("unknown FilingSessionStatus variant: %q", tag)
	}

	return nil, fmt.Errorf("expected a string for FilingSessionStatus")
}

// An error returned by the get filing session operation.
//
// GetFilingSessionError is one of [GetFilingSessionErrorNotFound]. Use a type
// switch to tell the variants apart.
type GetFilingSessionError interface {
	isGetFilingSessionError()
}

// The session was not found.
type GetFilingSessionErrorNotFound struct{}

func (GetFilingSessionErrorNotFound) isGetFilingSessionError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v GetFilingSessionErrorNotFound) MarshalJSON() ([]byte, error) {
	return marshalTagged("code", "not_found", struct{}{})
}

// decodeGetFilingSessionError decodes a [GetFilingSessionError] from its wire representation.
func decodeGetFilingSessionError(data []byte) (GetFilingSessionError, error) {
	var tag struct {
		Tag string `json:"code"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "not_found":
		return GetFilingSessionErrorNotFound{}, nil
	}
	return nil, fmt.Errorf("unknown GetFilingSessionError tag: %q", tag.Tag)
}

// An error returned by the list filing sessions operation.
//
// ListFilingSessionsError is one of [ListFilingSessionsErrorInvalidLimit]. Use
// a type switch to tell the variants apart.
type ListFilingSessionsError interface {
	isListFilingSessionsError()
}

// The provided limit is out of bounds.
type ListFilingSessionsErrorInvalidLimit struct {
	// The minimum limit.
	Min uint8 `json:"min"`
	// The maximum limit.
	Max uint8 `json:"max"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ListFilingSessionsErrorInvalidLimit) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"min", "max"}, nil); err != nil {
		return err
	}
	type plain ListFilingSessionsErrorInvalidLimit
	return json.Unmarshal(data, (*plain)(v))
}

func (ListFilingSessionsErrorInvalidLimit) isListFilingSessionsError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ListFilingSessionsErrorInvalidLimit) MarshalJSON() ([]byte, error) {
	type plain ListFilingSessionsErrorInvalidLimit
	return marshalTagged("code", "invalid_limit", plain(v))
}

// decodeListFilingSessionsError decodes a [ListFilingSessionsError] from its wire representation.
func decodeListFilingSessionsError(data []byte) (ListFilingSessionsError, error) {
	var tag struct {
		Tag string `json:"code"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "invalid_limit":
		var v ListFilingSessionsErrorInvalidLimit
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("unknown ListFilingSessionsError tag: %q", tag.Tag)
}

// ResumeFilingSessionError is one of [ResumeFilingSessionErrorSessionExpired].
// Use a type switch to tell the variants apart.
type ResumeFilingSessionError interface {
	isResumeFilingSessionError()
}

// The session has expired.
type ResumeFilingSessionErrorSessionExpired struct{}

func (ResumeFilingSessionErrorSessionExpired) isResumeFilingSessionError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ResumeFilingSessionErrorSessionExpired) MarshalJSON() ([]byte, error) {
	return marshalTagged("code", "session_expired", struct{}{})
}

// decodeResumeFilingSessionError decodes a [ResumeFilingSessionError] from its wire representation.
func decodeResumeFilingSessionError(data []byte) (ResumeFilingSessionError, error) {
	var tag struct {
		Tag string `json:"code"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "session_expired":
		return ResumeFilingSessionErrorSessionExpired{}, nil
	}
	return nil, fmt.Errorf("unknown ResumeFilingSessionError tag: %q", tag.Tag)
}

// ApplyFilingSessionEventError is one of
// [ApplyFilingSessionEventErrorNotFound],
// [ApplyFilingSessionEventErrorSessionBusy],
// [ApplyFilingSessionEventErrorStaleState],
// [ApplyFilingSessionEventErrorSessionExpired]. Use a type switch to tell the
// variants apart.
type ApplyFilingSessionEventError interface {
	isApplyFilingSessionEventError()
}

// No session exists for the given session id.
type ApplyFilingSessionEventErrorNotFound struct{}

func (ApplyFilingSessionEventErrorNotFound) isApplyFilingSessionEventError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ApplyFilingSessionEventErrorNotFound) MarshalJSON() ([]byte, error) {
	return marshalTagged("code", "not_found", struct{}{})
}

// A concurrent request for this session is already in progress.
type ApplyFilingSessionEventErrorSessionBusy struct{}

func (ApplyFilingSessionEventErrorSessionBusy) isApplyFilingSessionEventError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ApplyFilingSessionEventErrorSessionBusy) MarshalJSON() ([]byte, error) {
	return marshalTagged("code", "session_busy", struct{}{})
}

// The event was based on an outdated session state.
type ApplyFilingSessionEventErrorStaleState struct {
	// The current form.
	Form Form `json:"form"`
	// The current state version.
	StateVersion int64 `json:"state_version"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *ApplyFilingSessionEventErrorStaleState) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"form", "state_version"}, nil); err != nil {
		return err
	}
	type plain ApplyFilingSessionEventErrorStaleState
	return json.Unmarshal(data, (*plain)(v))
}

func (ApplyFilingSessionEventErrorStaleState) isApplyFilingSessionEventError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ApplyFilingSessionEventErrorStaleState) MarshalJSON() ([]byte, error) {
	type plain ApplyFilingSessionEventErrorStaleState
	return marshalTagged("code", "stale_state", plain(v))
}

// The session has expired.
type ApplyFilingSessionEventErrorSessionExpired struct{}

func (ApplyFilingSessionEventErrorSessionExpired) isApplyFilingSessionEventError() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ApplyFilingSessionEventErrorSessionExpired) MarshalJSON() ([]byte, error) {
	return marshalTagged("code", "session_expired", struct{}{})
}

// decodeApplyFilingSessionEventError decodes a [ApplyFilingSessionEventError] from its wire representation.
func decodeApplyFilingSessionEventError(data []byte) (ApplyFilingSessionEventError, error) {
	var tag struct {
		Tag string `json:"code"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "not_found":
		return ApplyFilingSessionEventErrorNotFound{}, nil
	case "session_busy":
		return ApplyFilingSessionEventErrorSessionBusy{}, nil
	case "stale_state":
		var v ApplyFilingSessionEventErrorStaleState
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "session_expired":
		return ApplyFilingSessionEventErrorSessionExpired{}, nil
	}
	return nil, fmt.Errorf("unknown ApplyFilingSessionEventError tag: %q", tag.Tag)
}

// Event is one of [EventSetField], [EventAction], [EventSearch]. Use a type
// switch to tell the variants apart.
type Event interface {
	isEvent()
}

type EventSetField struct {
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *EventSetField) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"field"}, []string{"value"}); err != nil {
		return err
	}
	type plain EventSetField
	return json.Unmarshal(data, (*plain)(v))
}

func (EventSetField) isEvent() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v EventSetField) MarshalJSON() ([]byte, error) {
	type plain EventSetField
	return marshalTagged("kind", "set_field", plain(v))
}

type EventAction struct {
	ActionID string `json:"action_id"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *EventAction) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"action_id"}, nil); err != nil {
		return err
	}
	type plain EventAction
	return json.Unmarshal(data, (*plain)(v))
}

func (EventAction) isEvent() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v EventAction) MarshalJSON() ([]byte, error) {
	type plain EventAction
	return marshalTagged("kind", "action", plain(v))
}

// Searches the values of a field that supports search.
type EventSearch struct {
	// The key of the field to search.
	Field string `json:"field"`
	// The text to search for. If empty, the server returns a default selection of
	// results.
	Query string `json:"query"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *EventSearch) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"field", "query"}, nil); err != nil {
		return err
	}
	type plain EventSearch
	return json.Unmarshal(data, (*plain)(v))
}

func (EventSearch) isEvent() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v EventSearch) MarshalJSON() ([]byte, error) {
	type plain EventSearch
	return marshalTagged("kind", "search", plain(v))
}

// decodeEvent decodes a [Event] from its wire representation.
func decodeEvent(data []byte) (Event, error) {
	var tag struct {
		Tag string `json:"kind"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "set_field":
		var v EventSetField
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "action":
		var v EventAction
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "search":
		var v EventSearch
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("unknown Event tag: %q", tag.Tag)
}

// A piece of content in a Block.
//
// Item is one of [ItemField], [ItemData], [ItemText]. Use a type switch to tell
// the variants apart.
type Item interface {
	isItem()
}

// An input field.
type ItemField struct {
	// Value is the value of the variant.
	Value Field
}

func (ItemField) isItem() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ItemField) MarshalJSON() ([]byte, error) {
	return marshalTagged("kind", "field", v.Value)
}

// A labeled piece of data.
type ItemData struct {
	// Value is the value of the variant.
	Value Data
}

func (ItemData) isItem() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ItemData) MarshalJSON() ([]byte, error) {
	return marshalTagged("kind", "data", v.Value)
}

// A passage of explanatory text.
type ItemText struct {
	// Value is the value of the variant.
	Value Text
}

func (ItemText) isItem() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ItemText) MarshalJSON() ([]byte, error) {
	return marshalTagged("kind", "text", v.Value)
}

// decodeItem decodes a [Item] from its wire representation.
func decodeItem(data []byte) (Item, error) {
	var tag struct {
		Tag string `json:"kind"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "field":
		var v ItemField
		if err := json.Unmarshal(data, &v.Value); err != nil {
			return nil, err
		}
		return v, nil
	case "data":
		var v ItemData
		if err := json.Unmarshal(data, &v.Value); err != nil {
			return nil, err
		}
		return v, nil
	case "text":
		var v ItemText
		if err := json.Unmarshal(data, &v.Value); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("unknown Item tag: %q", tag.Tag)
}

// The type of a field value.
//
// ValueType is one of [ValueTypeString], [ValueTypeNumber], [ValueTypeBool],
// [ValueTypeFile]. Use a type switch to tell the variants apart.
type ValueType interface {
	isValueType()
}

// A UTF-8 string.
type ValueTypeString struct{}

func (ValueTypeString) isValueType() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ValueTypeString) MarshalJSON() ([]byte, error) {
	return json.Marshal("string")
}

// A 64-bit floating point number.
type ValueTypeNumber struct{}

func (ValueTypeNumber) isValueType() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ValueTypeNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal("number")
}

// A boolean.
type ValueTypeBool struct{}

func (ValueTypeBool) isValueType() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ValueTypeBool) MarshalJSON() ([]byte, error) {
	return json.Marshal("bool")
}

// A document.
//
// It is represented as an object with the following fields:
//
// - file_name (string): The original file name - content_type (string): The
// MIME type of the document - size_bytes (number): The size of the document, in
// bytes - content (string): The base64-encoded document content
//
// The content field is required on write and omitted on read.
type ValueTypeFile struct{}

func (ValueTypeFile) isValueType() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v ValueTypeFile) MarshalJSON() ([]byte, error) {
	return json.Marshal("file")
}

// decodeValueType decodes a [ValueType] from its wire representation.
func decodeValueType(data []byte) (ValueType, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return nil, err
		}
		switch tag {
		case "string":
			return ValueTypeString{}, nil
		case "number":
			return ValueTypeNumber{}, nil
		case "bool":
			return ValueTypeBool{}, nil
		case "file":
			return ValueTypeFile{}, nil
		}
		return nil, fmt.Errorf("unknown ValueType variant: %q", tag)
	}

	return nil, fmt.Errorf("expected a string for ValueType")
}

// A rule that a value must satisfy.
//
// Rule is one of [RulePattern], [RuleExtension]. Use a type switch to tell the
// variants apart.
type Rule interface {
	isRule()
}

// The value must match a pattern.
type RulePattern struct {
	// The pattern that the input must match.
	//
	// This must be a valid regular expression using the JavaScript regex syntax.
	Pattern string `json:"pattern"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *RulePattern) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"pattern"}, nil); err != nil {
		return err
	}
	type plain RulePattern
	return json.Unmarshal(data, (*plain)(v))
}

func (RulePattern) isRule() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v RulePattern) MarshalJSON() ([]byte, error) {
	type plain RulePattern
	return marshalTagged("kind", "pattern", plain(v))
}

// The value must be a file with one of these extensions.
type RuleExtension struct {
	// The accepted file extensions, lowercase and without a leading dot, such as
	// `pdf`.
	Extensions []string `json:"extensions"`
}

// UnmarshalJSON implements [json.Unmarshaler]. It rejects data that is missing
// a required field.
func (v *RuleExtension) UnmarshalJSON(data []byte) error {
	if err := requireFields(data, []string{"extensions"}, nil); err != nil {
		return err
	}
	type plain RuleExtension
	return json.Unmarshal(data, (*plain)(v))
}

func (RuleExtension) isRule() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v RuleExtension) MarshalJSON() ([]byte, error) {
	type plain RuleExtension
	return marshalTagged("kind", "extension", plain(v))
}

// decodeRule decodes a [Rule] from its wire representation.
func decodeRule(data []byte) (Rule, error) {
	var tag struct {
		Tag string `json:"kind"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return nil, err
	}

	switch tag.Tag {
	case "pattern":
		var v RulePattern
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	case "extension":
		var v RuleExtension
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("unknown Rule tag: %q", tag.Tag)
}

// The browser autofill value represented by a field.
// https://html.spec.whatwg.org/multipage/form-control-infrastructure.html#autofill
//
// This is intentionally separate from [`TextInputMode`]: autofill describes the
// meaning of the value, not the keyboard used to enter it.
//
// Autocomplete is one of [AutocompleteUsername], [AutocompleteCurrentPassword],
// [AutocompleteNewPassword], [AutocompleteGivenName],
// [AutocompleteAdditionalName], [AutocompleteFamilyName], [AutocompleteTel],
// [AutocompleteAddressLine1], [AutocompleteAddressLine2], [AutocompleteCity],
// [AutocompleteState], [AutocompletePostalCode],
// [AutocompleteBillingAddressLine1], [AutocompleteBillingAddressLine2],
// [AutocompleteBillingCity], [AutocompleteBillingState],
// [AutocompleteBillingPostalCode], [AutocompleteCreditCardName],
// [AutocompleteCreditCardNumber], [AutocompleteCreditCardExpirationMonth],
// [AutocompleteCreditCardExpirationYear], [AutocompleteCreditCardSecurityCode].
// Use a type switch to tell the variants apart.
type Autocomplete interface {
	isAutocomplete()
}

type AutocompleteUsername struct{}

func (AutocompleteUsername) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteUsername) MarshalJSON() ([]byte, error) {
	return json.Marshal("username")
}

type AutocompleteCurrentPassword struct{}

func (AutocompleteCurrentPassword) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCurrentPassword) MarshalJSON() ([]byte, error) {
	return json.Marshal("current-password")
}

type AutocompleteNewPassword struct{}

func (AutocompleteNewPassword) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteNewPassword) MarshalJSON() ([]byte, error) {
	return json.Marshal("new-password")
}

type AutocompleteGivenName struct{}

func (AutocompleteGivenName) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteGivenName) MarshalJSON() ([]byte, error) {
	return json.Marshal("given-name")
}

type AutocompleteAdditionalName struct{}

func (AutocompleteAdditionalName) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteAdditionalName) MarshalJSON() ([]byte, error) {
	return json.Marshal("additional-name")
}

type AutocompleteFamilyName struct{}

func (AutocompleteFamilyName) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteFamilyName) MarshalJSON() ([]byte, error) {
	return json.Marshal("family-name")
}

type AutocompleteTel struct{}

func (AutocompleteTel) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteTel) MarshalJSON() ([]byte, error) {
	return json.Marshal("tel")
}

type AutocompleteAddressLine1 struct{}

func (AutocompleteAddressLine1) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteAddressLine1) MarshalJSON() ([]byte, error) {
	return json.Marshal("address-line1")
}

type AutocompleteAddressLine2 struct{}

func (AutocompleteAddressLine2) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteAddressLine2) MarshalJSON() ([]byte, error) {
	return json.Marshal("address-line2")
}

type AutocompleteCity struct{}

func (AutocompleteCity) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCity) MarshalJSON() ([]byte, error) {
	return json.Marshal("address-level2")
}

type AutocompleteState struct{}

func (AutocompleteState) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteState) MarshalJSON() ([]byte, error) {
	return json.Marshal("address-level1")
}

type AutocompletePostalCode struct{}

func (AutocompletePostalCode) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompletePostalCode) MarshalJSON() ([]byte, error) {
	return json.Marshal("postal-code")
}

type AutocompleteBillingAddressLine1 struct{}

func (AutocompleteBillingAddressLine1) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteBillingAddressLine1) MarshalJSON() ([]byte, error) {
	return json.Marshal("billing address-line1")
}

type AutocompleteBillingAddressLine2 struct{}

func (AutocompleteBillingAddressLine2) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteBillingAddressLine2) MarshalJSON() ([]byte, error) {
	return json.Marshal("billing address-line2")
}

type AutocompleteBillingCity struct{}

func (AutocompleteBillingCity) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteBillingCity) MarshalJSON() ([]byte, error) {
	return json.Marshal("billing address-level2")
}

type AutocompleteBillingState struct{}

func (AutocompleteBillingState) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteBillingState) MarshalJSON() ([]byte, error) {
	return json.Marshal("billing address-level1")
}

type AutocompleteBillingPostalCode struct{}

func (AutocompleteBillingPostalCode) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteBillingPostalCode) MarshalJSON() ([]byte, error) {
	return json.Marshal("billing postal-code")
}

type AutocompleteCreditCardName struct{}

func (AutocompleteCreditCardName) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCreditCardName) MarshalJSON() ([]byte, error) {
	return json.Marshal("cc-name")
}

type AutocompleteCreditCardNumber struct{}

func (AutocompleteCreditCardNumber) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCreditCardNumber) MarshalJSON() ([]byte, error) {
	return json.Marshal("cc-number")
}

type AutocompleteCreditCardExpirationMonth struct{}

func (AutocompleteCreditCardExpirationMonth) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCreditCardExpirationMonth) MarshalJSON() ([]byte, error) {
	return json.Marshal("cc-exp-month")
}

type AutocompleteCreditCardExpirationYear struct{}

func (AutocompleteCreditCardExpirationYear) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCreditCardExpirationYear) MarshalJSON() ([]byte, error) {
	return json.Marshal("cc-exp-year")
}

type AutocompleteCreditCardSecurityCode struct{}

func (AutocompleteCreditCardSecurityCode) isAutocomplete() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v AutocompleteCreditCardSecurityCode) MarshalJSON() ([]byte, error) {
	return json.Marshal("cc-csc")
}

// decodeAutocomplete decodes a [Autocomplete] from its wire representation.
func decodeAutocomplete(data []byte) (Autocomplete, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return nil, err
		}
		switch tag {
		case "username":
			return AutocompleteUsername{}, nil
		case "current-password":
			return AutocompleteCurrentPassword{}, nil
		case "new-password":
			return AutocompleteNewPassword{}, nil
		case "given-name":
			return AutocompleteGivenName{}, nil
		case "additional-name":
			return AutocompleteAdditionalName{}, nil
		case "family-name":
			return AutocompleteFamilyName{}, nil
		case "tel":
			return AutocompleteTel{}, nil
		case "address-line1":
			return AutocompleteAddressLine1{}, nil
		case "address-line2":
			return AutocompleteAddressLine2{}, nil
		case "address-level2":
			return AutocompleteCity{}, nil
		case "address-level1":
			return AutocompleteState{}, nil
		case "postal-code":
			return AutocompletePostalCode{}, nil
		case "billing address-line1":
			return AutocompleteBillingAddressLine1{}, nil
		case "billing address-line2":
			return AutocompleteBillingAddressLine2{}, nil
		case "billing address-level2":
			return AutocompleteBillingCity{}, nil
		case "billing address-level1":
			return AutocompleteBillingState{}, nil
		case "billing postal-code":
			return AutocompleteBillingPostalCode{}, nil
		case "cc-name":
			return AutocompleteCreditCardName{}, nil
		case "cc-number":
			return AutocompleteCreditCardNumber{}, nil
		case "cc-exp-month":
			return AutocompleteCreditCardExpirationMonth{}, nil
		case "cc-exp-year":
			return AutocompleteCreditCardExpirationYear{}, nil
		case "cc-csc":
			return AutocompleteCreditCardSecurityCode{}, nil
		}
		return nil, fmt.Errorf("unknown Autocomplete variant: %q", tag)
	}

	return nil, fmt.Errorf("expected a string for Autocomplete")
}

// The virtual keyboard a browser should prefer for a text field.
//
// TextInputMode is one of [TextInputModeText], [TextInputModeNumeric],
// [TextInputModeDecimal], [TextInputModeTel], [TextInputModeEmail]. Use a type
// switch to tell the variants apart.
type TextInputMode interface {
	isTextInputMode()
}

type TextInputModeText struct{}

func (TextInputModeText) isTextInputMode() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v TextInputModeText) MarshalJSON() ([]byte, error) {
	return json.Marshal("text")
}

type TextInputModeNumeric struct{}

func (TextInputModeNumeric) isTextInputMode() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v TextInputModeNumeric) MarshalJSON() ([]byte, error) {
	return json.Marshal("numeric")
}

type TextInputModeDecimal struct{}

func (TextInputModeDecimal) isTextInputMode() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v TextInputModeDecimal) MarshalJSON() ([]byte, error) {
	return json.Marshal("decimal")
}

type TextInputModeTel struct{}

func (TextInputModeTel) isTextInputMode() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v TextInputModeTel) MarshalJSON() ([]byte, error) {
	return json.Marshal("tel")
}

type TextInputModeEmail struct{}

func (TextInputModeEmail) isTextInputMode() {}

// MarshalJSON implements [json.Marshaler]. It writes the tag of the variant.
func (v TextInputModeEmail) MarshalJSON() ([]byte, error) {
	return json.Marshal("email")
}

// decodeTextInputMode decodes a [TextInputMode] from its wire representation.
func decodeTextInputMode(data []byte) (TextInputMode, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return nil, err
		}
		switch tag {
		case "text":
			return TextInputModeText{}, nil
		case "numeric":
			return TextInputModeNumeric{}, nil
		case "decimal":
			return TextInputModeDecimal{}, nil
		case "tel":
			return TextInputModeTel{}, nil
		case "email":
			return TextInputModeEmail{}, nil
		}
		return nil, fmt.Errorf("unknown TextInputMode variant: %q", tag)
	}

	return nil, fmt.Errorf("expected a string for TextInputMode")
}
