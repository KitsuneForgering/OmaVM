package gui

// catalogEntry is presentation-only data for the "what do you want to
// run?" picker (UX Principle 4 in CLAUDE.md: never ask which backend).
// It is a static, hardcoded list — not a Template system. The Template
// format is an intentionally open decision (see CLAUDE.md) and this
// catalog must not be read as resolving it.
type catalogEntry struct {
	Label string
	Icon  string // GTK icon name; freedesktop distributor-logo-* icons for real distros
	Image string // pre-filled Backend image reference; empty means the user supplies one

	// ForceMachine is true for non-Linux systems: Kind is fixed to
	// Machine and the Box/Machine picker is hidden entirely, per the
	// "Linux = container, non-Linux = VM" rule being about defaults,
	// not a ban on Linux Machines elsewhere in the app. It also means
	// the image field is a boot ISO path, not a container image
	// reference.
	ForceMachine bool
}

var catalog = []catalogEntry{
	{Label: "Fedora", Icon: "distributor-logo-fedora", Image: "fedora:latest"},
	{Label: "Ubuntu", Icon: "distributor-logo-ubuntu", Image: "ubuntu:latest"},
	{Label: "Debian", Icon: "distributor-logo-debian", Image: "debian:latest"},
	{Label: "Arch Linux", Icon: "distributor-logo-archlinux", Image: "archlinux:latest"},
	{Label: "Alpine", Icon: "distributor-logo-alpine", Image: "alpine:latest"},
	{Label: "Other Linux image…", Icon: "package-x-generic-symbolic"},
	{Label: "Other system (ISO)…", Icon: "media-optical-symbolic", ForceMachine: true},
}
