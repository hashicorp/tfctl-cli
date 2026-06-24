## Description

<!-- Describe a clear reason for this change -->

### Example Output

<!--
One easy way to create a screenshot is:
go run github.com/homeport/termshot/cmd/termshot@v0.6.1 -c -f ~/screenshot.png -- tfctl mycommand --myflag
-->

### PR Checklist

- [ ] Ensure you have [changie](https://changie.dev/guide/installation/) installed for release notes prep.
- [ ] Ensure any command changes are sensitive to these global flags:
  - `--json` &mdash; Force machine readable output to stdout. Does not apply to stderr.
  - `--markdown` &mdash; Force markdown output to stdout. Does not apply to stderr.
  - `--dry-run` &mdash; Don't make any actual writes or other mutations. Describe what would have changed to stderr.
  - `--quiet` &mdash; Don't render output to stdout.
- [ ] Get the logging interface from the context and add debug logging for interesting conditions and nonfatal situations.
- [ ] Run `make gen/screenshot` if the root command output changes.
- [ ] Add the `Autocomplete` field to **positional arguments** and **flags** to assist shell autocomplete.
- [ ] Run `changie new` to prepare a new changelog entry for the next set of release notes.

## PCI review checklist

<!-- heimdall_github_prtemplate:grc-pci_dss-2024-01-05 -->

- [ ] I have documented a clear reason for, and description of, the change I am making.

- [ ] If applicable, I've documented a plan to revert these changes if they require more than reverting the pull request.

- [ ] If applicable, I've documented the impact of any changes to security controls.

  Examples of changes to security controls include using new access control methods, adding or removing logging pipelines, etc.
