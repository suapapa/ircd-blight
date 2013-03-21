package ircd

import (
	"testing"
)

var registerDispatchMessages = []struct {
	Hook    string
	Mask    ExecutionMask
	Message *Message
}{
	{
		Hook:    "test",
		Mask:    EMASK_REGISTRATION,
		Message: &Message{},
	},
	{
		Hook:    "test",
		Mask:    EMASK_USER,
		Message: &Message{},
	},
	{
		Hook:    "test",
		Mask:    EMASK_SERVER,
		Message: &Message{},
	},
	{
		Hook:    "test",
		Mask:    EMASK_USER | EMASK_REGISTRATION,
		Message: &Message{},
	},
}

var registerDispatchHooks = []struct {
	Hook        string
	Mask        ExecutionMask
	Constrain   CallConstraints
	Func        func(string, ExecutionMask, *Message)
	ExpectCalls int
}{
	{
		Hook:        "other",
		Mask:        EMASK_ANY,
		Constrain:   AnyArgs,
		Func:        func(string, ExecutionMask, *Message) {},
		ExpectCalls: 0,
	},
	{
		Hook:        "test",
		Mask:        EMASK_ANY,
		Constrain:   AnyArgs,
		Func:        func(string, ExecutionMask, *Message) {},
		ExpectCalls: 4,
	},
	{
		Hook:        "test",
		Mask:        EMAKS_USER | EMAKS_SERVER,
		Constrain:   AnyArgs,
		Func:        func(string, ExecutionMask, *Message) {},
		ExpectCalls: 2,
	},
}

func TestRegisterDispatch(t *testing.T) {
	/* TODO(kevlar): Reimplement this test
	hooks := make([]*Hook, len(registerDispatchHooks))

	for i, test := range registerDispatchHooks {
		hooks[i] = Register(test.Hook, test.Mask, test.Constrain, test.Func)
	}

	for _, disp := range registerDispatchMessages {
		Dispatch(disp.Hook, disp.Mask, disp.Message)
	}

	for i, test := range registerDispatchHooks {
		if got, want := hooks[i].Calls, test.ExpectCalls; got != want {
			t.Errorf("#%d: %d calls, want %d", i, got, want)
		}
	}
	*/
}
