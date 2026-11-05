package daytona

import (
	sdk "github.com/daytona/clients/sdk-go/pkg/daytona"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func enrichRecord(rec *Record, sb *sdk.Sandbox) {
	if rec == nil || sb == nil {
		return
	}
	enrichRecordCore(rec, sb)
	enrichRecordLifecycle(rec, sb)
	enrichRecordNetwork(rec, sb)
}

func enrichRecordCore(rec *Record, sb *sdk.Sandbox) {
	rec.RawState = string(sb.State)
	if sb.Snapshot != nil {
		rec.Snapshot = *sb.Snapshot
	}
	rec.User = sb.User
	rec.Public = sb.Public
	rec.Target = sb.Target
	rec.CPU = sb.Cpu
	rec.Memory = sb.Memory
	rec.Disk = sb.Disk
	rec.GPU = sb.Gpu
	rec.Spot = sb.Spot
	if sb.GpuType != nil {
		rec.GpuType = string(*sb.GpuType)
	}
	if sb.ErrorReason != nil {
		rec.ErrorReason = *sb.ErrorReason
	}
	if sb.DesiredState != nil {
		rec.DesiredState = string(*sb.DesiredState)
	}
	if sb.SandboxClass != nil {
		rec.SandboxClass = string(*sb.SandboxClass)
	}
	for _, v := range sb.Volumes {
		mount := v.MountPath
		if mount == "" {
			mount = v.VolumeId
		}
		if mount != "" {
			rec.VolumeMounts = append(rec.VolumeMounts, mount)
		}
	}
}

func enrichRecordLifecycle(rec *Record, sb *sdk.Sandbox) {
	rec.AutoStopInterval = sb.AutoStopInterval
	rec.AutoPauseInterval = sb.AutoPauseInterval
	rec.AutoArchiveInterval = sb.AutoArchiveInterval
	rec.AutoDeleteInterval = sb.AutoDeleteInterval
	if sb.AutoDestroyAt != nil {
		rec.AutoDestroyAt = *sb.AutoDestroyAt
	}
	if sb.LastActivityAt != nil {
		rec.LastActivityAt = *sb.LastActivityAt
	}
	if sb.UpdatedAt != nil {
		rec.UpdatedAt = *sb.UpdatedAt
	}
}

func enrichRecordNetwork(rec *Record, sb *sdk.Sandbox) {
	if sb.Kvm != nil {
		rec.Kvm = *sb.Kvm
	}
	if sb.NetworkBlockAll != nil {
		rec.NetworkBlockAll = *sb.NetworkBlockAll
	}
	if sb.NetworkAllowList != nil {
		rec.NetworkAllowList = *sb.NetworkAllowList
	}
	if sb.DomainAllowList != nil {
		rec.DomainAllowList = *sb.DomainAllowList
	}
}

func daytonaDetailFields(rec Record, url, auth string) []sandbox.Field {
	var fields []sandbox.Field
	fields = sandbox.AppendTextField(fields, "ID", rec.ID)
	fields = sandbox.AppendTextField(fields, "State", rec.RawState)
	fields = sandbox.AppendTextField(fields, "Snapshot", rec.Snapshot)
	fields = sandbox.AppendTextField(fields, "User", rec.User)
	fields = sandbox.AppendBoolField(fields, "Public", rec.Public)
	fields = sandbox.AppendTextField(fields, "Target", rec.Target)
	fields = sandbox.AppendIntField(fields, "CPU", int(rec.CPU))
	fields = sandbox.AppendIntField(fields, "Memory", int(rec.Memory))
	fields = sandbox.AppendIntField(fields, "Disk", int(rec.Disk))
	if rec.GPU > 0 {
		fields = sandbox.AppendIntField(fields, "GPU", int(rec.GPU))
		fields = sandbox.AppendTextField(fields, "GPU type", rec.GpuType)
		fields = sandbox.AppendBoolField(fields, "Spot", rec.Spot)
	}
	fields = sandbox.AppendTextField(fields, "Error", rec.ErrorReason)
	if rec.DesiredState != "" && rec.DesiredState != rec.RawState {
		fields = sandbox.AppendTextField(fields, "Desired state", rec.DesiredState)
	}
	fields = daytonaAppendInterval(fields, "Auto stop", rec.AutoStopInterval)
	fields = daytonaAppendInterval(fields, "Auto pause", rec.AutoPauseInterval)
	fields = daytonaAppendInterval(fields, "Auto archive", rec.AutoArchiveInterval)
	fields = daytonaAppendInterval(fields, "Auto delete", rec.AutoDeleteInterval)
	fields = sandbox.AppendTextField(fields, "Auto destroy", rec.AutoDestroyAt)
	fields = sandbox.AppendTextField(fields, "Last activity", rec.LastActivityAt)
	fields = sandbox.AppendTimeField(fields, "Created", rec.CreatedAt)
	fields = sandbox.AppendTextField(fields, "Updated", rec.UpdatedAt)
	fields = sandbox.AppendTextField(fields, "Class", rec.SandboxClass)
	for _, mount := range rec.VolumeMounts {
		fields = sandbox.AppendTextField(fields, "Volume", mount)
	}
	fields = sandbox.AppendBoolField(fields, "KVM", rec.Kvm)
	fields = sandbox.AppendBoolField(fields, "Network block", rec.NetworkBlockAll)
	fields = sandbox.AppendTextField(fields, "Network allow", rec.NetworkAllowList)
	fields = sandbox.AppendTextField(fields, "Domain allow", rec.DomainAllowList)
	fields = sandbox.AppendTextField(fields, "URL", url)
	fields = sandbox.AppendTextField(fields, "Auth", auth)
	return fields
}

func daytonaAppendInterval(fields []sandbox.Field, label string, minutes int) []sandbox.Field {
	if minutes == 0 {
		return fields
	}
	return sandbox.AppendFormattedField(fields, label, "%d min", minutes)
}
