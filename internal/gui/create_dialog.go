package gui

import (
	"context"
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// newCatalogTile builds one selectable tile (icon over label) for the
// "what do you want to run?" picker — a visual grid in the style of
// Parallels' Installation Assistant, rather than a plain text dropdown
// (design research, 2026-09-26; see CLAUDE.md Omarchy Integration).
func newCatalogTile(entry catalogEntry) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 6)
	box.SetMarginTop(10)
	box.SetMarginBottom(10)
	box.SetMarginStart(10)
	box.SetMarginEnd(10)
	box.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImageFromIconName(entry.Icon)
	icon.SetPixelSize(48)
	box.Append(icon)

	label := gtk.NewLabel(entry.Label)
	label.SetJustify(gtk.JustifyCenter)
	label.SetWrap(true)
	label.SetMaxWidthChars(12)
	box.Append(label)

	return box
}

// showCreateDialog implements UX Principle 4 from CLAUDE.md: never ask
// "which backend?" — ask "what do you want to run?", then, only for
// Linux distros where it's ambiguous, let the user pick Development
// Environment (Box) vs Virtual Machine (Machine). Non-Linux systems
// skip that question entirely: Kind is fixed to Machine.
func (ec *experienceCenter) showCreateDialog() {
	nameEntry := gtk.NewEntry()
	nameEntry.SetPlaceholderText("e.g. radic")

	customImageEntry := gtk.NewEntry()
	customImageEntry.SetPlaceholderText("container image, e.g. fedora:41")

	isoEntry := gtk.NewEntry()
	isoEntry.SetPlaceholderText("path to boot ISO")
	isoEntry.SetHExpand(true)
	browseBtn := gtk.NewButtonWithLabel("Browse…")
	browseBtn.ConnectClicked(func() {
		fd := gtk.NewFileDialog()
		fd.SetTitle("Choose a boot ISO")
		fd.Open(context.Background(), &ec.window.Window, func(res gio.AsyncResulter) {
			file, err := fd.OpenFinish(res)
			if err != nil || file == nil {
				return
			}
			if path := file.Path(); path != "" {
				isoEntry.SetText(path)
			}
		})
	})
	isoRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	isoRow.Append(isoEntry)
	isoRow.Append(browseBtn)

	kindLabel := gtk.NewLabel("Environment type:")
	kindLabel.SetHAlign(gtk.AlignStart)
	kindDropdown := gtk.NewDropDownFromStrings([]string{"Development Environment", "Virtual Machine"})

	askLabel := gtk.NewLabel("What do you want to run?")
	askLabel.SetHAlign(gtk.AlignStart)

	catalogFlow := gtk.NewFlowBox()
	catalogFlow.SetSelectionMode(gtk.SelectionBrowse)
	catalogFlow.SetHomogeneous(true)
	catalogFlow.SetRowSpacing(4)
	catalogFlow.SetColumnSpacing(4)
	catalogFlow.SetMinChildrenPerLine(4)
	catalogFlow.SetMaxChildrenPerLine(4)
	for _, entry := range catalog {
		catalogFlow.Append(newCatalogTile(entry))
	}

	nameLabel := gtk.NewLabel("Name:")
	nameLabel.SetHAlign(gtk.AlignStart)

	form := gtk.NewBox(gtk.OrientationVertical, 8)
	form.SetMarginTop(12)
	form.SetMarginBottom(12)
	form.SetMarginStart(12)
	form.SetMarginEnd(12)
	form.Append(askLabel)
	form.Append(catalogFlow)
	form.Append(customImageEntry)
	form.Append(isoRow)
	form.Append(kindLabel)
	form.Append(kindDropdown)
	form.Append(nameLabel)
	form.Append(nameEntry)

	selectedCatalogEntry := func() catalogEntry {
		selected := catalogFlow.SelectedChildren()
		if len(selected) == 0 {
			return catalog[0]
		}
		return catalog[selected[0].Index()]
	}

	updateVisibility := func() {
		entry := selectedCatalogEntry()
		customImageEntry.SetVisible(entry.Image == "" && !entry.ForceMachine)
		isoRow.SetVisible(entry.ForceMachine)
		showKind := !entry.ForceMachine
		kindLabel.SetVisible(showKind)
		kindDropdown.SetVisible(showKind)
	}
	catalogFlow.ConnectSelectedChildrenChanged(updateVisibility)
	catalogFlow.SelectChild(catalogFlow.ChildAtIndex(0))
	updateVisibility()

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	createBtn := gtk.NewButtonWithLabel("Create")
	createBtn.AddCSSClass("suggested-action")

	buttonRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	buttonRow.SetHAlign(gtk.AlignEnd)
	buttonRow.Append(cancelBtn)
	buttonRow.Append(createBtn)
	form.Append(buttonRow)

	dialog := adw.NewDialog()
	dialog.SetTitle("New Environment")
	dialog.SetContentWidth(460)
	dialog.SetChild(form)

	cancelBtn.ConnectClicked(func() { dialog.Close() })
	createBtn.ConnectClicked(func() {
		if nameEntry.Text() == "" {
			ec.notifyError(fmt.Errorf("please give the environment a name"))
			return
		}

		entry := selectedCatalogEntry()
		kind := core.Box
		image := entry.Image
		switch {
		case entry.ForceMachine:
			kind = core.Machine
			image = isoEntry.Text()
		case entry.Image == "":
			image = customImageEntry.Text()
			fallthrough
		default:
			if kindDropdown.Selected() == 1 {
				kind = core.Machine
			}
		}

		name := nameEntry.Text()
		dialog.Close()
		ec.createEnvironment(name, image, kind)
	})

	dialog.Present(ec.window)
}

func (ec *experienceCenter) createEnvironment(name, image string, kind core.EnvironmentKind) {
	go func() {
		_, err := ec.svc.Create(context.Background(), core.Environment{Name: name, Image: image, Kind: kind})
		glib.IdleAdd(func() {
			if err != nil {
				ec.notifyError(err)
				return
			}
			ec.notifyInfo("Environment Created", name+" is ready.")
			ec.refresh()
		})
	}()
}
