package reconcile

import (
	"context"
	"fmt"
	"strings"

	"github.com/prop4n/proxmops/internal/manifest"
	"github.com/prop4n/proxmops/internal/proxmox"
)

// guestReconciler reconciles QEMU VMs using tag-based ownership: only guests
// carrying proxmox.ManagedTag are updated or deleted, so hand-made guests are
// left untouched. Increment 1 covers VirtualMachine create/update/delete;
// containers are handled in a later phase.
type guestReconciler struct {
	store proxmox.GuestStore
}

// NewGuestReconciler returns a Reconciler for VMs and containers.
func NewGuestReconciler(store proxmox.GuestStore) Reconciler {
	return &guestReconciler{store: store}
}

// Plan diffs desired VMs against the cluster within the owned scope.
func (g *guestReconciler) Plan(ctx context.Context, desired []manifest.Resource) (Plan, error) {
	want := filterKinds(desired, manifest.KindVirtualMachine)

	observed, err := g.store.ListGuests(ctx)
	if err != nil {
		return Plan{}, err
	}

	// Guests are keyed by VMID, their stable Proxmox identity. Matching by name
	// would misfire right after a create, when the cluster resource cache still
	// reports an empty name for a few seconds, causing a spurious re-create.
	desiredByVMID := make(map[int]manifest.VirtualMachine, len(want))
	order := make([]int, 0, len(want))
	for _, r := range want {
		vm, ok := r.(manifest.VirtualMachine)
		if !ok {
			continue
		}
		desiredByVMID[vm.Spec.VMID] = vm
		order = append(order, vm.Spec.VMID)
	}

	// Templates are qemu guests too, but the templateReconciler owns them; skip
	// them here so a template is never seen as a VM to update or prune.
	observedByVMID := make(map[int]proxmox.Object, len(observed))
	readyTemplateVMIDs := make(map[int]bool)
	for _, o := range observed {
		if o.Kind == proxmox.KindVirtualMachine && !o.IsTemplate {
			observedByVMID[o.VMID] = o
		}
		if o.IsTemplate {
			readyTemplateVMIDs[o.VMID] = true
		}
	}

	// Declared templates, so a VM's fromTemplate name resolves to a vmid, and so
	// the delete scan can skip VMIDs a template owns while it is being built.
	templateVMIDs := make(map[string]int)
	templateVMIDByID := make(map[int]bool)
	for _, r := range filterKinds(desired, manifest.KindTemplate) {
		if tpl, ok := r.(manifest.Template); ok {
			templateVMIDs[tpl.Metadata.Name] = tpl.Spec.VMID
			templateVMIDByID[tpl.Spec.VMID] = true
		}
	}

	var plan Plan

	// Creates and updates, in manifest order.
	for _, vmid := range order {
		vm := desiredByVMID[vmid]
		obs, present := observedByVMID[vmid]
		if !present {
			// A clone must wait for its template to be built and converted; if the
			// template is declared but not yet a ready template on the cluster, defer
			// the clone to a later pass rather than racing the build.
			if ft := vm.Spec.FromTemplate; ft != nil {
				if tplVMID, ok := templateVMIDs[ft.Name]; ok && !readyTemplateVMIDs[tplVMID] {
					continue
				}
			}
			// Resolution errors (e.g. an undeclared template) surface as a failing
			// action rather than a silent skip.
			spec, specErr := desiredSpec(vm, templateVMIDs)
			apply := func(ctx context.Context) error { return g.store.CreateGuest(ctx, spec) }
			if specErr != nil {
				apply = func(context.Context) error { return specErr }
			}
			plan.Actions = append(plan.Actions, Action{
				Type:   ActionCreate,
				Kind:   manifest.KindVirtualMachine,
				Name:   vm.Metadata.Name,
				Reason: "not present in cluster",
				Apply:  apply,
			})
			continue
		}
		// An owned, present VM may have drifted on safe fields.
		if !obs.Owned() {
			continue
		}
		if reason, upd, drifted := guestDrift(vm, obs); drifted {
			plan.Actions = append(plan.Actions, Action{
				Type:   ActionUpdate,
				Kind:   manifest.KindVirtualMachine,
				Name:   vm.Metadata.Name,
				Reason: reason,
				Apply:  func(ctx context.Context) error { return g.store.UpdateGuest(ctx, upd) },
			})
			continue
		}
		// A changed user-data means a new cidata ISO. A stopped VM reads it on its
		// next boot, so it is re-provisioned right away. A running VM must reboot to
		// re-read it: that happens only with applyMode reboot, otherwise it is
		// reported and the old ISO is left in place until the reboot is authorised.
		if vm.Spec.UserData != "" && proxmox.CidataHash(vm.Spec.UserData) != obs.CidataHash {
			spec, specErr := desiredSpec(vm, templateVMIDs)
			node, id := obs.Node, obs.VMID
			reboot := obs.Running
			if reboot && vm.Spec.ApplyMode != manifest.ApplyModeReboot {
				plan.Actions = append(plan.Actions, Action{
					Type:          ActionUpdate,
					Kind:          manifest.KindVirtualMachine,
					Name:          vm.Metadata.Name,
					Reason:        "user-data changed, reboot required to apply (set applyMode: reboot)",
					Informational: true,
				})
				continue
			}
			reason := "user-data changed, reprovisioning"
			if reboot {
				reason = "user-data changed, reprovisioning and restarting"
			}
			plan.Actions = append(plan.Actions, Action{
				Type:   ActionUpdate,
				Kind:   manifest.KindVirtualMachine,
				Name:   vm.Metadata.Name,
				Reason: reason,
				Apply: func(ctx context.Context) error {
					if specErr != nil {
						return specErr
					}
					if err := g.store.SyncUserData(ctx, spec); err != nil {
						return err
					}
					if reboot {
						return g.store.RebootGuest(ctx, node, id)
					}
					return nil
				},
			})
			continue
		}
		// Config already matches desired, but a prior change to a running VM is
		// waiting on a restart. Reboot when opted in; otherwise just report it.
		if obs.RebootPending {
			if vm.Spec.ApplyMode == manifest.ApplyModeReboot {
				node, id := obs.Node, obs.VMID
				plan.Actions = append(plan.Actions, Action{
					Type:   ActionUpdate,
					Kind:   manifest.KindVirtualMachine,
					Name:   vm.Metadata.Name,
					Reason: "restarting to apply pending changes",
					Apply:  func(ctx context.Context) error { return g.store.RebootGuest(ctx, node, id) },
				})
			} else {
				plan.Actions = append(plan.Actions, Action{
					Type:          ActionUpdate,
					Kind:          manifest.KindVirtualMachine,
					Name:          vm.Metadata.Name,
					Reason:        "reboot required to apply pending changes (set applyMode: reboot to automate)",
					Informational: true,
				})
			}
		}
	}

	// Deletes: owned VMs absent from the desired set. Templates are excluded;
	// they belong to the templateReconciler. A VMID claimed by a declared
	// Template is skipped too, so a template still building (its VM exists but is
	// not yet converted) is not deleted as a stray VM.
	for _, o := range observed {
		if o.Kind != proxmox.KindVirtualMachine || o.IsTemplate || !o.Owned() {
			continue
		}
		if _, ok := desiredByVMID[o.VMID]; ok {
			continue
		}
		if _, ok := templateVMIDByID[o.VMID]; ok {
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Type:   ActionDelete,
			Kind:   manifest.Kind(o.Kind),
			Name:   o.Name,
			Reason: "removed from repository",
			Apply:  func(ctx context.Context) error { return g.store.DeleteGuest(ctx, o) },
		})
	}

	return plan, nil
}

// guestDrift compares the safe, non-destructive fields (cores, memory, power
// state) of a desired VM against the observed one. It returns a human reason,
// the update to apply, and whether anything drifted. Disk and NIC differences
// are out of scope for this increment and are not reported here.
func guestDrift(vm manifest.VirtualMachine, obs proxmox.Object) (string, proxmox.GuestUpdate, bool) {
	// Only manage fields the manifest actually declares: unspecified cores or
	// memory (0) and an unset power state are left as observed, so a partial
	// manifest never fights Proxmox defaults.
	upd := proxmox.GuestUpdate{Node: obs.Node, VMID: obs.VMID, Cores: obs.Cores, MemoryMB: obs.MemoryMB, CPU: obs.CPU, Running: obs.Running}
	var reasons []string
	if vm.Spec.Cores > 0 && vm.Spec.Cores != obs.Cores {
		reasons = append(reasons, fmt.Sprintf("cores %d->%d", obs.Cores, vm.Spec.Cores))
		upd.Cores = vm.Spec.Cores
	}
	if vm.Spec.CPU != "" && vm.Spec.CPU != obs.CPU {
		reasons = append(reasons, fmt.Sprintf("cpu %s->%s", obs.CPU, vm.Spec.CPU))
		upd.CPU = vm.Spec.CPU
	}
	if ci := vm.Spec.CloudInit; ci != nil {
		if ci.User != "" && ci.User != obs.CIUser {
			reasons = append(reasons, fmt.Sprintf("ci-user %s->%s", obs.CIUser, ci.User))
			upd.CIUser = ci.User
		}
		// Proxmox stores ipconfig as "ip=..."; normalise the desired value so a
		// bare "dhcp" doesn't read as perpetual drift against "ip=dhcp".
		if wantIP := normalizeIP(ci.IP); ci.IP != "" && wantIP != obs.IP {
			reasons = append(reasons, fmt.Sprintf("ip %s->%s", obs.IP, wantIP))
			upd.IP = wantIP
		}
		if ci.Nameserver != "" && ci.Nameserver != obs.Nameserver {
			reasons = append(reasons, fmt.Sprintf("nameserver %s->%s", obs.Nameserver, ci.Nameserver))
			upd.Nameserver = ci.Nameserver
		}
		if ci.SearchDomain != "" && ci.SearchDomain != obs.SearchDomain {
			reasons = append(reasons, fmt.Sprintf("searchdomain %s->%s", obs.SearchDomain, ci.SearchDomain))
			upd.SearchDomain = ci.SearchDomain
		}
	}
	if vm.Spec.Memory > 0 && vm.Spec.Memory != obs.MemoryMB {
		reasons = append(reasons, fmt.Sprintf("memory %d->%d MB", obs.MemoryMB, vm.Spec.Memory))
		upd.MemoryMB = vm.Spec.Memory
	}
	if vm.Spec.State != "" {
		wantRunning := vm.Spec.State == manifest.StateRunning
		if wantRunning != obs.Running {
			reasons = append(reasons, fmt.Sprintf("state %s->%s", powerWord(obs.Running), powerWord(wantRunning)))
			upd.Running = wantRunning
		}
	}
	if len(reasons) == 0 {
		return "", proxmox.GuestUpdate{}, false
	}
	return strings.Join(reasons, ", "), upd, true
}

// normalizeIP mirrors Proxmox's ipconfig form: a bare mode like "dhcp" becomes
// "ip=dhcp"; a full "ip=...,gw=..." is left as is.
func normalizeIP(ip string) string {
	if ip != "" && !strings.Contains(ip, "=") {
		return "ip=" + ip
	}
	return ip
}

func powerWord(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

// desiredSpec projects a manifest VM onto the flat create spec, adding the
// ownership tag the created guest must carry. templateVMIDs maps declared
// template names to their vmid, used to resolve a fromTemplate clone; a
// reference to an undeclared template is an error.
func desiredSpec(vm manifest.VirtualMachine, templateVMIDs map[string]int) (proxmox.GuestSpec, error) {
	tags := append([]string{}, vm.Metadata.Tags...)
	tags = append(tags, proxmox.ManagedTag)

	spec := proxmox.GuestSpec{
		Kind:     proxmox.KindVirtualMachine,
		Node:     vm.Metadata.Node,
		VMID:     vm.Spec.VMID,
		Name:     vm.Metadata.Name,
		Cores:    vm.Spec.Cores,
		MemoryMB: vm.Spec.Memory,
		CPU:      vm.Spec.CPU,
		ISO:      vm.Spec.ISO,
		UserData: vm.Spec.UserData,
		Running:  vm.Spec.State == manifest.StateRunning,
		Tags:     tags,
	}
	if len(vm.Spec.Disks) > 0 {
		spec.Disk = proxmox.GuestDisk{Storage: vm.Spec.Disks[0].Storage, Size: vm.Spec.Disks[0].Size, Bus: vm.Spec.Disks[0].Bus}
	}
	if len(vm.Spec.Net) > 0 {
		spec.NIC = proxmox.GuestNIC{Bridge: vm.Spec.Net[0].Bridge, Model: vm.Spec.Net[0].Model}
	}
	if vm.Spec.Image != nil {
		spec.Image = &proxmox.GuestImage{
			Source:        vm.Spec.Image.Source,
			Filename:      vm.Spec.Image.Filename(),
			ImportStorage: vm.Spec.Image.ImportStorage,
		}
	}
	if ft := vm.Spec.FromTemplate; ft != nil {
		tplVMID, ok := templateVMIDs[ft.Name]
		if !ok {
			return proxmox.GuestSpec{}, fmt.Errorf("template %q is not declared", ft.Name)
		}
		spec.Clone = &proxmox.GuestClone{TemplateVMID: tplVMID, Full: !ft.Linked}
	}
	if ci := vm.Spec.CloudInit; ci != nil {
		spec.CloudInit = &proxmox.GuestCloudInit{
			User:         ci.User,
			Password:     ci.Password,
			SSHKeys:      ci.SSHKeys,
			IP:           ci.IP,
			Nameserver:   ci.Nameserver,
			SearchDomain: ci.SearchDomain,
		}
	}
	return spec, nil
}
