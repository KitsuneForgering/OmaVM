package gui

import (
	"context"
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// isolationLabel spells out the real isolation guarantee behind a Kind.
// CLAUDE.md's Security Model forbids presenting a Box as if it offered
// VM-equivalent isolation, so this distinction is always visible on the
// card, not just in documentation.
func isolationLabel(k core.EnvironmentKind) string {
	switch k {
	case core.Box:
		return "Box — shares host kernel"
	case core.Machine:
		return "Machine — independent kernel (VM)"
	default:
		return "unknown"
	}
}

// newActionButton builds an icon+label button via AdwButtonContent, the
// standard GNOME/Adwaita pattern, instead of a text-only button.
func newActionButton(iconName, label string) *gtk.Button {
	content := adw.NewButtonContent()
	content.SetIconName(iconName)
	content.SetLabel(label)
	btn := gtk.NewButton()
	btn.SetChild(content)
	return btn
}

const previewHeight = 120

// newPreview builds the card's screenshot area. Only a Machine can have
// one — a Box has no display to capture, and CLAUDE.md's Security Model
// forbids papering over that real difference, so a Box gets a plain,
// honest label instead of a picture pretending equivalence.
func (ec *experienceCenter) newPreview(env core.Environment) gtk.Widgetter {
	if env.Kind != core.Machine {
		label := gtk.NewLabel("Box — no display to preview")
		label.AddCSSClass("dim-label")
		label.AddCSSClass("caption")
		label.SetVAlign(gtk.AlignCenter)
		label.SetHAlign(gtk.AlignCenter)
		label.SetSizeRequest(-1, previewHeight)
		return label
	}

	picture := gtk.NewPicture()
	picture.SetContentFit(gtk.ContentFitCover)
	picture.SetCanShrink(true)
	picture.SetSizeRequest(-1, previewHeight)
	picture.SetVisible(false)
	picture.AddCSSClass("card")

	placeholder := gtk.NewLabel("Machine stopped — no preview")
	placeholder.AddCSSClass("dim-label")
	placeholder.AddCSSClass("caption")
	placeholder.SetVAlign(gtk.AlignCenter)
	placeholder.SetHAlign(gtk.AlignCenter)
	placeholder.SetSizeRequest(-1, previewHeight)

	stack := gtk.NewBox(gtk.OrientationVertical, 0)
	stack.Append(picture)
	stack.Append(placeholder)

	go func() {
		path, err := ec.svc.Preview(context.Background(), env.Name)
		glib.IdleAdd(func() {
			if err != nil {
				placeholder.SetVisible(true)
				picture.SetVisible(false)
				return
			}
			// A fresh file at the same path: force the Picture to
			// actually reread it rather than assume an unchanged path
			// means unchanged content.
			picture.SetFilename("")
			picture.SetFilename(path)
			picture.SetVisible(true)
			placeholder.SetVisible(false)
		})
	}()

	return stack
}

func (ec *experienceCenter) newCard(env core.Environment) gtk.Widgetter {
	outer := gtk.NewBox(gtk.OrientationVertical, 8)
	outer.AddCSSClass("card")
	outer.SetMarginTop(12)
	outer.SetMarginBottom(12)
	outer.SetMarginStart(12)
	outer.SetMarginEnd(12)

	outer.Append(ec.newPreview(env))

	nameLabel := gtk.NewLabel(env.Name)
	nameLabel.AddCSSClass("title-3")
	nameLabel.SetHAlign(gtk.AlignStart)

	isolationL := gtk.NewLabel(isolationLabel(env.Kind))
	isolationL.AddCSSClass("dim-label")
	isolationL.AddCSSClass("caption")
	isolationL.SetHAlign(gtk.AlignStart)

	imageL := gtk.NewLabel(env.Image)
	imageL.SetHAlign(gtk.AlignStart)
	imageL.SetWrap(true)

	statusL := gtk.NewLabel("checking status…")
	statusL.AddCSSClass("dim-label")
	statusL.SetHAlign(gtk.AlignStart)

	outer.Append(nameLabel)
	outer.Append(isolationL)
	outer.Append(imageL)
	outer.Append(statusL)

	startBtn := newActionButton("media-playback-start-symbolic", "Start")
	openBtn := newActionButton("window-new-symbolic", "Open")
	stopBtn := newActionButton("media-playback-stop-symbolic", "Stop")
	removeBtn := newActionButton("user-trash-symbolic", "Delete")
	removeBtn.AddCSSClass("destructive-action")

	grid := gtk.NewGrid()
	grid.SetColumnSpacing(6)
	grid.SetRowSpacing(6)
	grid.SetColumnHomogeneous(true)
	grid.Attach(startBtn, 0, 0, 1, 1)
	grid.Attach(openBtn, 1, 0, 1, 1)
	grid.Attach(stopBtn, 0, 1, 1, 1)
	grid.Attach(removeBtn, 1, 1, 1, 1)
	outer.Append(grid)

	all := []*gtk.Button{startBtn, openBtn, stopBtn, removeBtn}
	runAction := func(verb string, action func(context.Context) error) {
		ec.log.Info("action", "verb", verb, "environment", env.Name)
		for _, b := range all {
			b.SetSensitive(false)
		}
		go func() {
			err := action(context.Background())
			glib.IdleAdd(func() {
				for _, b := range all {
					b.SetSensitive(true)
				}
				if err != nil {
					ec.notifyError(err)
					return
				}
				ec.refresh()
			})
		}()
	}

	startBtn.ConnectClicked(func() {
		runAction("start", func(ctx context.Context) error { return ec.svc.Start(ctx, env.Name) })
	})
	stopBtn.ConnectClicked(func() {
		runAction("stop", func(ctx context.Context) error { return ec.svc.Stop(ctx, env.Name) })
	})
	openBtn.ConnectClicked(func() {
		if env.Kind == core.Box {
			// A Box's Open attaches an interactive shell; the GUI
			// process has no terminal of its own to attach to.
			runAction("open", func(context.Context) error { return openInTerminal(env.Name) })
			return
		}
		runAction("open", func(ctx context.Context) error { return ec.svc.Open(ctx, env.Name) })
	})
	removeBtn.ConnectClicked(func() {
		ec.confirmDelete(env.Name, func() {
			runAction("remove", func(ctx context.Context) error { return ec.svc.Remove(ctx, env.Name) })
		})
	})

	go func() {
		status, err := ec.svc.Status(context.Background(), env.Name)
		glib.IdleAdd(func() {
			if err != nil {
				statusL.SetText("status unavailable")
				return
			}
			statusL.SetText(string(status.State))
		})
	}()

	return outer
}

// confirmDelete asks before a destructive action via a native Adwaita
// alert, rather than deleting immediately.
func (ec *experienceCenter) confirmDelete(name string, onConfirm func()) {
	d := adw.NewAlertDialog("Delete Environment", fmt.Sprintf("Delete %q? This cannot be undone.", name))
	d.AddResponse("cancel", "Cancel")
	d.AddResponse("delete", "Delete")
	d.SetResponseAppearance("delete", adw.ResponseDestructive)
	d.SetDefaultResponse("cancel")
	d.SetCloseResponse("cancel")
	d.ConnectResponse(func(response string) {
		if response == "delete" {
			onConfirm()
		}
	})
	d.Present(ec.window)
}
