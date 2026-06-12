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
