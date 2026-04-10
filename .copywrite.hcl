schema_version = 1

project {
  license        = "MPL-2.0"
  copyright_year = 2026

	# (OPTIONAL) If true, ignore updating the first year (start year) in copyright ranges.
	# End-year logic remains unchanged.
	# Default: false
	# ignore_year1 = false

  # (OPTIONAL) A list of globs that should not have copyright/license headers.
  # Supports doublestar glob patterns for more flexibility in defining which
  # files or folders should be ignored
  header_ignore = [
    # "vendor/**",
    # "**autogen**",
  ]
}
