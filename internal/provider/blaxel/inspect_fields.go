package blaxel

import (
	"fmt"
	"strings"

	sdk "github.com/blaxel-ai/sdk-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func blaxelDetailFields(sb *sdk.Sandbox, url, auth string) []sandbox.Field {
	if sb == nil {
		return nil
	}
	var fields []sandbox.Field
	if sb.State != "" {
		fields = sandbox.AppendTextField(fields, "State", string(sb.State))
	}
	fields = sandbox.AppendTextField(fields, "Workspace", sb.Metadata.Workspace)
	fields = sandbox.AppendTextField(fields, "Region", sb.Spec.Region)
	if sb.Spec.Vpc != "" && sb.Spec.Vpc != "default" {
		fields = sandbox.AppendTextField(fields, "VPC", sb.Spec.Vpc)
	}
	fields = sandbox.AppendTextField(fields, "Image", sb.Spec.Runtime.Image)
	fields = sandbox.AppendInt64Field(fields, "Memory", sb.Spec.Runtime.Memory)
	fields = sandbox.AppendTextField(fields, "TTL", sb.Spec.Runtime.Ttl)
	fields = sandbox.AppendTextField(fields, "Expires", sb.Spec.Runtime.Expires)
	if sb.ExpiresIn > 0 {
		fields = sandbox.AppendFormattedField(fields, "Expires in", "%d s", sb.ExpiresIn)
	}
	fields = append(fields, blaxelLifecycleFields(sb.Spec.Lifecycle)...)
	for _, p := range sb.Spec.Runtime.Ports {
		if p.Target == 0 {
			continue
		}
		label := "Port"
		if p.Name != "" {
			label = p.Name
		}
		text := fmt.Sprintf("%s %d", p.Protocol, p.Target)
		fields = sandbox.AppendTextField(fields, label, text)
	}
	for _, v := range sb.Spec.Volumes {
		fields = sandbox.AppendTextField(fields, "Volume", v.Name)
	}
	fields = sandbox.AppendTextField(fields, "Created", sb.Metadata.CreatedAt)
	fields = sandbox.AppendTextField(fields, "Updated", sb.Metadata.UpdatedAt)
	fields = sandbox.AppendTextField(fields, "Last used", sb.LastUsedAt)
	for _, e := range sb.Errors {
		msg := e.Message
		if e.Code != "" {
			msg = e.Code + ": " + msg
		}
		fields = sandbox.AppendTextField(fields, "Error", strings.TrimSpace(msg))
	}
	fields = sandbox.AppendTextField(fields, "URL", url)
	fields = sandbox.AppendTextField(fields, "Auth", auth)
	return fields
}

func blaxelLifecycleFields(lc sdk.SandboxLifecycle) []sandbox.Field {
	var fields []sandbox.Field
	for _, p := range lc.ExpirationPolicies {
		text := string(p.Type)
		if p.Value != "" {
			text += " " + p.Value
		}
		if p.Action != "" {
			text += " (" + string(p.Action) + ")"
		}
		fields = sandbox.AppendTextField(fields, "Lifecycle", text)
	}
	fields = sandbox.AppendTextField(fields, "Terminated retention", lc.TerminatedRetention)
	return fields
}
