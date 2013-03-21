package ircd

// Choose in what contexts a hook is called
type ExecutionMask int

const (
	EMASK_REGISTRATION ExecutionMask = 1 << iota
	EMASK_USER
	EMASK_SERVER
	EMASK_ANY ExecutionMask = EMASK_REGISTRATION | EMASK_USER | EMASK_SERVER
)

// Choose how many arguments a hook needs to be called
type CallConstraints struct {
	MinArgs int
	MaxArgs int
}

func NArgs(count int) CallConstraints {
	return CallConstraints{
		MinArgs: count,
		MaxArgs: count,
	}
}

func MinArgs(min int) CallConstraints {
	return CallConstraints{
		MinArgs: min,
		MaxArgs: -1,
	}
}

func OptArgs(required, optional int) CallConstraints {
	return CallConstraints{
		MinArgs: required,
		MaxArgs: required + optional,
	}
}

var (
	AnyArgs = CallConstraints{
		MinArgs: 0,
		MaxArgs: -1,
	}
)

// Allow registration of hooks in any module
type Hook struct {
	When        ExecutionMask
	Constraints CallConstraints
	Calls       int
	Func        func(hook string, message *Message, ircd *IRCd)
}

var (
	registeredHooks = map[string][]*Hook{}
)

func Register(hook string, when ExecutionMask, args CallConstraints,
	fn func(string, *Message, *IRCd)) *Hook {
	if _, ok := registeredHooks[hook]; !ok {
		registeredHooks[hook] = make([]*Hook, 0, 1)
	}
	h := &Hook{
		When:        when,
		Constraints: args,
		Func:        fn,
	}
	registeredHooks[hook] = append(registeredHooks[hook], h)
	return h
}

// TODO(kevlar): Add channel to send messages back on
func DispatchClient(message *Message, ircd *IRCd) {
	hookName := message.Command
	_, _, _, reg, ok := GetUserInfo(message.SenderID)
	if !ok {
		panic("Unknown user: " + message.SenderID)
	}
	var mask ExecutionMask
	switch reg {
	case UnregisteredUser:
		mask |= EMASK_REGISTRATION
	case RegisteredAsUser:
		mask |= EMASK_USER
	}
	for _, hook := range registeredHooks[hookName] {
		if hook.When&mask == mask {
			// TODO(kevlar): Check callconstraints
			go hook.Func(hookName, message, ircd)
			hook.Calls++
		}
	}
}

func DispatchServer(message *Message, ircd *IRCd) {
	hookName := message.Command
	_, _, _, reg, ok := GetServerInfo(message.SenderID)
	if !ok {
		Warn.Printf("Unknown source server: %s", message.SenderID)
		return
	}
	var mask ExecutionMask
	switch reg {
	case UnregisteredServer:
		mask |= EMASK_REGISTRATION
	case RegisteredAsServer:
		mask |= EMASK_SERVER
	}
	for _, hook := range registeredHooks[hookName] {
		if hook.When&mask == mask {
			// TODO(kevlar): Check callconstraints
			go hook.Func(hookName, message, ircd)
			hook.Calls++
		}
	}
}
