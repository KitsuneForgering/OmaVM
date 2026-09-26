// Package gui is the OmaVM Experience Center: a GTK4 + libadwaita front
// end that calls the same core.Service the CLI calls. It holds no
// environment lifecycle logic of its own — every action here is a thin
// call into the Core, never a direct shell/infrastructure command
// (CLAUDE.md, CLI/GUI Contract). GTK4/libadwaita is used, not a
// toolkit-agnostic framework, so the window follows Omarchy's native
// GNOME/Adwaita look (theme, dark mode, accent color) automatically.
package gui

import (
	"context"
	"log/slog"
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/KitsuneSemCalda/OmaVM/internal/core"
)

// experienceCenter is the main window: environments appear as simple
// cards (name, image, kind, status) with actions, never as raw
// backend/infrastructure details (UX Principles 3, 5).
type experienceCenter struct {
	svc    *core.Service
	log    *slog.Logger
	app    *adw.Application
	window *adw.ApplicationWindow
	flow   *gtk.FlowBox
	toast  *adw.ToastOverlay
}

// Run builds and shows the Experience Center. It blocks until the
// application quits. logger must not be installed as slog's global
// default by the caller — see internal/applog's doc comment for why.
func Run(svc *core.Service, logger *slog.Logger) {
	ec := &experienceCenter{svc: svc, log: logger}
	ec.app = adw.NewApplication("dev.omavm.app", gio.ApplicationFlagsNone)
	ec.app.ConnectActivate(func() { ec.activate() })
	os.Exit(ec.app.Run(os.Args))
}

func (ec *experienceCenter) activate() {
	ec.window = adw.NewApplicationWindow(&ec.app.Application)
	ec.window.SetTitle("OmaVM — Experience Center")
	ec.window.SetDefaultSize(920, 620)

	newBtn := gtk.NewButtonFromIconName("list-add-symbolic")
	newBtn.SetTooltipText("New Environment")
	newBtn.ConnectClicked(func() { ec.showCreateDialog() })

	refreshBtn := gtk.NewButtonFromIconName("view-refresh-symbolic")
	refreshBtn.SetTooltipText("Refresh")
	refreshBtn.ConnectClicked(func() { ec.refresh() })

	header := adw.NewHeaderBar()
	header.PackStart(newBtn)
	header.PackEnd(refreshBtn)
	header.SetTitleWidget(adw.NewWindowTitle("OmaVM", "Experience Center"))

	ec.flow = gtk.NewFlowBox()
	ec.flow.SetSelectionMode(gtk.SelectionNone)
	ec.flow.SetHomogeneous(true)
	ec.flow.SetRowSpacing(12)
	ec.flow.SetColumnSpacing(12)
	ec.flow.SetMinChildrenPerLine(1)
	ec.flow.SetMaxChildrenPerLine(4)
	ec.flow.SetMarginTop(12)
	ec.flow.SetMarginBottom(12)
	ec.flow.SetMarginStart(12)
	ec.flow.SetMarginEnd(12)
	ec.flow.SetVAlign(gtk.AlignStart)
	ec.flow.SetVExpand(true)

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(ec.flow)
	scroll.SetVExpand(true)

	ec.toast = adw.NewToastOverlay()
	ec.toast.SetChild(scroll)

	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(header)
	toolbar.SetContent(ec.toast)

	ec.window.SetContent(toolbar)
	ec.window.Present()

	ec.refresh()
}

// notifyError surfaces a Core/backend error as a transient in-window
// toast and, since the window may not be focused or visible when a
// background action finishes, also as a real desktop notification —
// notifications are a first-class Omarchy integration point per
// CLAUDE.md, not an afterthought.
func (ec *experienceCenter) notifyError(err error) {
	ec.log.Error("action failed", "error", err)

	toast := adw.NewToast(err.Error())
	toast.SetTimeout(6)
	ec.toast.AddToast(toast)

	n := gio.NewNotification("OmaVM")
	n.SetBody(err.Error())
	n.SetPriority(gio.NotificationPriorityHigh)
	ec.app.SendNotification("omavm-error", n)
}

// notifyInfo is the non-error counterpart, used for lifecycle events
// worth a desktop notification (e.g. an environment finished being
// created) without the urgency styling of notifyError.
func (ec *experienceCenter) notifyInfo(title, body string) {
	ec.log.Info(title, "detail", body)

	n := gio.NewNotification(title)
	n.SetBody(body)
	ec.app.SendNotification("omavm-info", n)
}

// refresh reloads the environment list from the Core and rebuilds the
// cards. It runs the (potentially slow, backend-shelling-out) List call
// off the GTK main loop and applies the result via glib.IdleAdd.
func (ec *experienceCenter) refresh() {
	go func() {
		envs, err := ec.svc.List(context.Background())
		glib.IdleAdd(func() {
			if err != nil {
				ec.notifyError(err)
				return
			}
			ec.flow.RemoveAll()
			if len(envs) == 0 {
				ec.flow.Append(ec.newEmptyState())
				return
			}
			for _, env := range envs {
				ec.flow.Append(ec.newCard(env))
			}
		})
	}()
}

// newEmptyState is the idiomatic Adwaita pattern for "nothing here yet"
// (AdwStatusPage), replacing a plain label with something that matches
// the rest of the Omarchy/GNOME app ecosystem.
func (ec *experienceCenter) newEmptyState() gtk.Widgetter {
	page := adw.NewStatusPage()
	page.SetIconName("computer-symbolic")
	page.SetTitle("No Environments Yet")
	page.SetDescription("Create a Box for a Linux userspace, or a Machine for anything that needs its own kernel.")

	createBtn := gtk.NewButtonWithLabel("New Environment")
	createBtn.AddCSSClass("suggested-action")
	createBtn.AddCSSClass("pill")
	createBtn.SetHAlign(gtk.AlignCenter)
	createBtn.ConnectClicked(func() { ec.showCreateDialog() })
	page.SetChild(createBtn)

	return page
}
