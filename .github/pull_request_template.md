## Description

<!-- Describe a clear reason for this change -->

## PCI review checklist

<!-- heimdall_github_prtemplate:grc-pci_dss-2024-01-05 -->

- [ ] I have documented a clear reason for, and description of, the change I am making.

- [x] If applicable, I've documented a plan to revert these changes if they require more than reverting the pull request.

- [ ] If applicable, I've documented the impact of any changes to security controls.

If you have any questions, please contact your direct supervisor, GRC (#team-grc), or the PCI working group (#proj-pci-reboot). You can also find more information at [PCI Compliance](https://hashicorp.atlassian.net/wiki/spaces/SEC/pages/2784559202/PCI+Compliance).

## The Three Ex's:

### External Links

- [JIRA](https://hashicorp.atlassian.net/browse/xxxx)

### Example Output

<!--
One easy way to create a screenshot is:
go run github.com/homeport/termshot/cmd/termshot@v0.6.1 -c -f ~/screenshot.png -- tfctl mycommand --myflag
-->

### Extra Things that are Easy to Forget

- [ ] If you added a top level command or global flag, run `make gen/screenshot` to update the README screenshot
- [ ] Think through bash autocomplete prediction for new command targets or flag argument values
