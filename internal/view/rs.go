package view

import (
	"errors"
	"fmt"

	"github.com/derailed/k9s/internal/client"
	"github.com/derailed/k9s/internal/dao"
	"github.com/derailed/k9s/internal/model"
	"github.com/derailed/k9s/internal/ui"
	"github.com/derailed/tcell/v2"
	"github.com/derailed/tview"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReplicaSet presents a replicaset viewer.
type ReplicaSet struct {
	ResourceViewer
}

// NewReplicaSet returns a new viewer.
func NewReplicaSet(gvr client.GVR) ResourceViewer {
	r := ReplicaSet{
		ResourceViewer: NewBrowser(gvr),
	}
	r.AddBindKeysFn(r.bindKeys)
	r.GetTable().SetEnterFn(r.showPods)

	return &r
}

func (r *ReplicaSet) bindKeys(aa ui.KeyActions) {
	aa.Add(ui.KeyActions{
		ui.KeyO:        ui.NewKeyAction("Show Owner", r.showOwner, true),
		ui.KeyShiftD:   ui.NewKeyAction("Sort Desired", r.GetTable().SortColCmd("DESIRED", true), false),
		ui.KeyShiftC:   ui.NewKeyAction("Sort Current", r.GetTable().SortColCmd("CURRENT", true), false),
		ui.KeyShiftR:   ui.NewKeyAction("Sort Ready", r.GetTable().SortColCmd(readyCol, true), false),
		tcell.KeyCtrlL: ui.NewKeyAction("Rollback", r.rollbackCmd, true),
	})
}

func (r *ReplicaSet) showPods(app *App, model ui.Tabular, gvr, path string) {
	var drs dao.ReplicaSet
	rs, err := drs.Load(app.factory, path)
	if err != nil {
		app.Flash().Err(err)
		return
	}

	showPodsFromSelector(app, path, rs.Spec.Selector)
}

func (r *ReplicaSet) showOwner(evt *tcell.EventKey) *tcell.EventKey {
	path := r.GetTable().GetSelectedItem()
	if path == "" {
		return evt
	}
	rs, err := fetchRs(r.App().factory, path)
	if err != nil {
		r.App().Flash().Err(err)
		return nil
	}
	if len(rs.GetObjectMeta().GetOwnerReferences()) == 0 {
		r.App().Flash().Err(errors.New("no owner reference found"))
		return nil
	}

	var owner model.Component
	for _, ownerReference := range rs.GetObjectMeta().GetOwnerReferences() {
		if ownerReference.Kind == "Deployment" {
			dp := NewDeploy(client.NewGVR("apps/v1/deployments"))
			dp.SetInstance(rs.Namespace + "/" + dp.Name())

			owner = dp
			break
		}
	}

	if owner == nil {
		r.App().Flash().Err(errors.New("no Deployment owner reference found"))
		return nil
	}

	if err := r.App().inject(owner, false); err != nil {
		r.App().Flash().Err(err)
	}

	return nil
}

func (r *ReplicaSet) rollbackCmd(evt *tcell.EventKey) *tcell.EventKey {
	path := r.GetTable().GetSelectedItem()
	if path == "" {
		return evt
	}

	r.showModal(fmt.Sprintf("Rollback %s %s?", r.GVR(), path), func(_ int, button string) {
		defer r.dismissModal()

		if button != "OK" {
			return
		}
		r.App().Flash().Infof("Rolling back %s %s", r.GVR(), path)
		var drs dao.ReplicaSet
		drs.Init(r.App().factory, r.GVR())
		if err := drs.Rollback(path); err != nil {
			r.App().Flash().Err(err)
		} else {
			r.App().Flash().Infof("%s successfully rolled back", path)
		}
		r.Refresh()
	})

	return nil
}

func (r *ReplicaSet) dismissModal() {
	r.App().Content.RemovePage("confirm")
}

func (r *ReplicaSet) showModal(msg string, done func(int, string)) {
	styles := r.App().Styles.Dialog()
	confirm := tview.NewModal().
		AddButtons([]string{"Cancel", "OK"}).
		SetButtonBackgroundColor(styles.ButtonBgColor.Color()).
		SetTextColor(tcell.ColorFuchsia).
		SetText(msg).
		SetDoneFunc(done)
	r.App().Content.AddPage("confirm", confirm, false, false)
	r.App().Content.ShowPage("confirm")
}

func fetchRs(f dao.Factory, path string) (*v1.ReplicaSet, error) {
	o, err := f.Get("apps/v1/replicasets", path, true, labels.Everything())
	if err != nil {
		return nil, err
	}

	var rs v1.ReplicaSet
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(o.(*unstructured.Unstructured).Object, &rs)
	if err != nil {
		return nil, err
	}

	return &rs, nil
}
