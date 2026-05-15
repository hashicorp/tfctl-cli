package harness

import (
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/skills"
)

// InstallOpts defines the options for the `harness install` command.
type InstallOpts struct {
	IO        iostreams.IOStreams
	Profile   *profile.Profile
	Output    *format.Outputter
	AgentName string
	Global    bool
	DryRun    bool
}

// NewCmdHarnessInstall creates the `harness install` command.
func NewCmdHarnessInstall(ctx *cmd.Context) *cmd.Command {
	installOpts := InstallOpts{
		IO:      ctx.IO,
		Profile: ctx.Profile,
		Output:  ctx.Output,
	}

	cmd := &cmd.Command{
		Name:      "install",
		ShortHelp: "Install coding agent skills for tfctl.",
		LongHelp: heredoc.New(ctx.IO, heredoc.WithPreserveNewlines()).Mustf(`
		The {{ template "mdCodeOrBold" "%s harness install" }} command installs the official tfctl agent skill for the selected platform. The available agent platforms are: {{ template "mdCodeOrBold" "%s" }}.

		Alternatively, you can use npx skills to install the tfctl skill for most other agents:

		{{ Color "green" "$ npx skills add hashicorp/tfctl-cli --skill 'tfctl'" }}
		`, config.Name, strings.Join(skills.ListAgents(), ", ")),
		Examples: []cmd.Example{
			{
				Preamble: "Install in the project directory for opencode and many other agents:",
				Command:  "$ tfctl harness install opencode",
			},
			{
				Preamble: "Install in the global user directory for codex:",
				Command:  "$ tfctl harness install codex --global",
			},
		},
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "AGENT",
					Documentation: heredoc.New(ctx.IO).Mustf(`The agent to install the skill for. Valid options are {{ template "mdCodeOrBold" "%s" }}`, strings.Join(skills.ListAgents(), ", ")),
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:          "global",
					Description:   "Install the skill to the agent's global config directory.",
					IsBooleanFlag: true,
					Value:         flagvalue.Simple(false, &installOpts.Global),
				},
			},
		},
		RunF: func(c *cmd.Command, args []string) error {
			installOpts.AgentName = args[0]

			if ctx.IsDryRun() {
				installOpts.DryRun = true
			}

			return runInstall(&installOpts)
		},
	}

	return cmd
}

func runInstall(opts *InstallOpts) error {
	agent, ok := skills.GetAgent(opts.AgentName)
	if !ok {
		return fmt.Errorf("invalid agent %q, valid options are: %s", opts.AgentName, strings.Join(skills.ListAgents(), ", "))
	}

	if opts.DryRun {
		if opts.Global {
			fmt.Fprintf(opts.IO.Err(), "%s Would create skill for %s to global directory: %s/%s\n", opts.IO.ColorScheme().DryRunLabel(), agent.DisplayName, agent.GlobalSkillsDir(), skills.TFCTLSkillPath)
		} else {
			fmt.Fprintf(opts.IO.Err(), "%s Would create skill for %s to project directory: %s/%s\n", opts.IO.ColorScheme().DryRunLabel(), agent.DisplayName, agent.SkillsDir, skills.TFCTLSkillPath)
		}
		return nil
	}

	predicate := fmt.Sprintf("to project directory %s", agent.SkillsDir)
	if opts.Global {
		predicate = fmt.Sprintf("to global directory %s", agent.GlobalSkillsDir())
	}

	err := agent.InstallSkill(opts.Global)
	if err != nil {
		return fmt.Errorf("failed to install skill for agent %q: %w", opts.AgentName, err)
	}
	fmt.Fprintf(opts.IO.Err(), "%s Successfully installed %s for %s %s\n", opts.IO.ColorScheme().SuccessIcon(), skills.TFCTLSkillPath, agent.DisplayName, predicate)
	return nil
}
