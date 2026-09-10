package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/svpchain/svpchain-agent/internal/agent/a2amcp"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

// ConnectTool attaches a remote agent's tool surface to the running
// conversation. It is the step between "search_agents found one" and "call its
// build_* tool": until it runs, a discovered agent is just a URL.
const ConnectTool = "a2a_connect_agent"

// attached is what a run picks up mid-conversation. A pointer on dispatchEnv,
// because the tool that fills it runs inside the tool loop and the next LLM
// round has to see the result.
//
// Precedence is enforced here and nowhere else: a tool name already served
// locally or by the remote MCP is dropped, never overridden. The endpoint
// these tools come from was named by a search index, so it must not be able to
// displace the signer or the trusted builder by advertising their names.
type attached struct {
	mu       sync.Mutex
	client   toolSource
	tools    []llm.Tool
	reserved map[string]bool
	// served is the filtered set — what this run will actually route to the
	// agent. Dispatch must read THIS, never the agent's own advertised list,
	// or a name dropped for colliding with a local or remote tool would still
	// reach the agent when the model called it.
	served map[string]bool
	// onAttach reports a successful attach so the caller can persist the URL.
	onAttach func(url string, tools []string)
	// lostURL / lostErr record a re-attach that failed at the start of the run,
	// so a call to one of that agent's tools gets a useful error instead of
	// the remote-MCP-is-off one.
	lostURL string
	lostErr error
}

// toolSource is an attached agent as this file uses it. *a2amcp.Client is the
// only implementation; the interface exists so the precedence rules below can
// be tested without a live endpoint.
type toolSource interface {
	URL() string
	Tools() []llm.Tool
	Handles(name string) bool
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

func newAttached(base []llm.Tool) *attached {
	reserved := make(map[string]bool, len(base)+1)
	for _, t := range base {
		if n := strings.TrimSpace(t.Function.Name); n != "" {
			reserved[n] = true
		}
	}
	reserved[ConnectTool] = true
	return &attached{reserved: reserved}
}

// tools returns the tools added this run, to be appended to the base list.
func (a *attached) toolDefs() []llm.Tool {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]llm.Tool(nil), a.tools...)
}

func (a *attached) handles(name string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client != nil && a.served[strings.TrimSpace(name)]
}

// mayServe checks whether a freshly discovered source can contribute name
// without colliding with a local or otherwise reserved tool. It is used before
// renewing settlement for a capability remembered from an earlier turn.
func (a *attached) mayServe(source toolSource, name string) bool {
	if a == nil || source == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" || !source.Handles(name) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.reserved[name]
}

// endpoint returns the currently attached A2A endpoint. It is used by the
// settlement gate immediately before dispatching an attached agent's tool.
func (a *attached) endpoint() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client == nil {
		return ""
	}
	return a.client.URL()
}

// retrier exposes the attached client as an authRetrier when its transport can
// run the handshake. *a2amcp.Client does; a test fake need not, so a nil result
// simply means "call it, but do not try to re-authenticate".
func (a *attached) retrier() authRetrier {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if r, ok := a.client.(authRetrier); ok {
		return r
	}
	return nil
}

func (a *attached) call(ctx context.Context, name string, args map[string]any) (string, error) {
	name = strings.TrimSpace(name)
	a.mu.Lock()
	client, served := a.client, a.served[name]
	a.mu.Unlock()
	if client == nil {
		return "", fmt.Errorf("no remote agent is attached; call %s first", ConnectTool)
	}
	// Belt and braces with handles(): the agent is reachable only for the
	// names this run accepted from it.
	if !served {
		return "", fmt.Errorf("the attached agent does not serve %q in this run", name)
	}
	return client.CallTool(ctx, name, args)
}

// set records the connected client and the tools it contributes, dropping any
// name that is already taken. Returns the names actually gained.
func (a *attached) set(client toolSource) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.client = client
	a.tools = nil
	a.served = map[string]bool{}
	var names []string
	for _, t := range client.Tools() {
		name := strings.TrimSpace(t.Function.Name)
		if name == "" || a.reserved[name] {
			continue
		}
		a.tools = append(a.tools, t)
		a.served[name] = true
		names = append(names, name)
	}
	return names
}

// noteReattachFailure records that the agent at url, attached in an earlier
// turn, could not be re-attached this run.
func (a *attached) noteReattachFailure(url string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lostURL, a.lostErr = url, err
}

// lost returns the agent that failed to re-attach, if any.
func (a *attached) lost() (string, error) {
	if a == nil {
		return "", nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lostURL, a.lostErr
}

// skipped reports the advertised names this run refused to take, so the
// assistant can say so instead of silently using the local one.
func (a *attached) skipped(client toolSource) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []string
	for _, t := range client.Tools() {
		if name := strings.TrimSpace(t.Function.Name); name != "" && a.reserved[name] {
			out = append(out, name)
		}
	}
	return out
}

type connectResult struct {
	AgentURL       string   `json:"agent_url"`
	Authenticated  bool     `json:"authenticated,omitempty"`
	ToolsAvailable []string `json:"tools_available"`
	ToolsSkipped   []string `json:"tools_skipped,omitempty"`
	Note           string   `json:"note,omitempty"`
}

// connect discovers a remote agent's tools, authenticates against it with the
// local key, and makes those tools callable for the rest of the conversation.
func (env dispatchEnv) connect(ctx context.Context, args map[string]any) (string, error) {
	url := ""
	if args != nil {
		if v, ok := args["agent_url"].(string); ok {
			url = strings.TrimSpace(v)
		}
	}
	if url == "" {
		return "", fmt.Errorf("agent_url is required")
	}
	if env.att == nil {
		return "", fmt.Errorf("this run cannot attach remote agents")
	}

	client := a2amcp.New(url)
	if err := client.Connect(ctx); err != nil {
		return "", err
	}

	res := connectResult{AgentURL: url}
	// The caller address is data for remote EVM transaction construction, not a
	// credential. Its local signer still verifies payload.signer_address before
	// signing the resulting transaction.
	if env.local != nil {
		client.SetCaller(env.local.Owner())
	} else {
		res.Note = "no local signer; read-only tools remain available, but transaction builders need a caller address"
	}

	// Legacy direct-A2A bearer handshake. Keep the implementation in a2amcp for
	// now, but do not execute it: private DeFi MCP access is authenticated by the
	// EVM Agent's shared internal token instead.
	//
	// if env.local != nil {
	// 	if err := client.EnsureAuth(ctx, env.local.Owner(), env.local.SignChallenge); err != nil {
	// 		res.Note = "not authenticated: " + err.Error()
	// 	} else {
	// 		res.Authenticated = true
	// 	}
	// }

	res.ToolsSkipped = env.att.skipped(client)
	res.ToolsAvailable = env.att.set(client)
	if len(res.ToolsAvailable) == 0 {
		return "", fmt.Errorf(
			"agent %s advertised only tool names that are already served locally or by the remote MCP", url)
	}
	env.att.mu.Lock()
	env.att.lostURL, env.att.lostErr = "", nil
	onAttach := env.att.onAttach
	env.att.mu.Unlock()
	if onAttach != nil {
		onAttach(url, res.ToolsAvailable)
	}

	bz, err := json.Marshal(res)
	if err != nil {
		return "", err
	}
	return string(bz), nil
}

// notePaidConnect rewrites the connect tool's description once settlement is
// configured. In that mode connecting an agent that has no task active in this
// run funds one, so the model must not read attaching as a free lookup and must
// not call it again "just to check" after paying.
func notePaidConnect(tools []llm.Tool) {
	for i := range tools {
		if strings.TrimSpace(tools[i].Function.Name) != ConnectTool {
			continue
		}
		tools[i].Function.Description += " Paid settlement is enabled in this conversation: " +
			"if the agent has no settlement task active in this conversation, connecting first pays its " +
			"advertised price (local approval and deposit confirmations), exactly as " +
			BeginSettlementTool + " would. Connecting an agent this conversation already funded for a behavior " +
			"that has not been reported complete costs nothing."
	}
}

// ConnectToolDef is offered whenever agent search is available: finding an
// agent is only useful if its tools can then be attached.
func ConnectToolDef() llm.Tool {
	return llm.Tool{
		Type: "function",
		Function: llm.Function{
			Name: ConnectTool,
			Description: "Attach a remote svpchain agent found with search_agents, making its tools callable for " +
				"the rest of this conversation. Discovers what it serves and authenticates with the local key. " +
				"Use when the tool needed for a task is not in your tool list. The attached tools keep their " +
				"normal names and follow the normal build → sign locally → broadcast flow.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent_url": map[string]any{
						"type":        "string",
						"description": "The endpoint from a search_agents result",
					},
				},
				"required": []string{"agent_url"},
			},
		},
	}
}
