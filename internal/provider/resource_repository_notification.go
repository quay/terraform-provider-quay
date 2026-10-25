package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/quay/terraform-provider-quay/internal/client"
)

var _ resource.Resource = &RepositoryNotificationResource{}
var _ resource.ResourceWithImportState = &RepositoryNotificationResource{}
var _ resource.ResourceWithConfigValidators = &RepositoryNotificationResource{}

type RepositoryNotificationResource struct {
	client *client.Client
}

type RepositoryNotificationResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Namespace        types.String `tfsdk:"namespace"`
	Name             types.String `tfsdk:"name"`
	UUID             types.String `tfsdk:"uuid"`
	Title            types.String `tfsdk:"title"`
	NumberOfFailures types.Int64  `tfsdk:"number_of_failures"`

	Webhook          *WebhookConfig          `tfsdk:"webhook"`
	Slack            *SlackConfig            `tfsdk:"slack"`
	Email            *EmailConfig            `tfsdk:"email"`
	QuayNotification *QuayNotificationConfig `tfsdk:"quay_notification"`

	RepoPush              *RepoPushEvent              `tfsdk:"repo_push"`
	BuildQueued           *BuildEvent                 `tfsdk:"build_queued"`
	BuildStart            *BuildEvent                 `tfsdk:"build_start"`
	BuildSuccess          *BuildEvent                 `tfsdk:"build_success"`
	BuildFailure          *BuildEvent                 `tfsdk:"build_failure"`
	BuildCancelled        *BuildEvent                 `tfsdk:"build_cancelled"`
	VulnerabilityFound    *VulnerabilityFoundEvent    `tfsdk:"vulnerability_found"`
	RepoMirrorSyncStarted *RepoMirrorSyncStartedEvent `tfsdk:"repo_mirror_sync_started"`
	RepoMirrorSyncSuccess *RepoMirrorSyncSuccessEvent `tfsdk:"repo_mirror_sync_success"`
	RepoMirrorSyncFailed  *RepoMirrorSyncFailedEvent  `tfsdk:"repo_mirror_sync_failed"`
	RepoImageExpiry       *RepoImageExpiryEvent       `tfsdk:"repo_image_expiry"`
}

type WebhookConfig struct {
	URL types.String `tfsdk:"url"`
}

type SlackConfig struct {
	URL types.String `tfsdk:"url"`
}

type EmailConfig struct {
	Email types.String `tfsdk:"email"`
}

// QuayNotificationTarget identifies the Quay user or team to notify in-app.
// Quay returns 400 "Missing target" when config.target is absent.
type QuayNotificationTarget struct {
	Kind types.String `tfsdk:"kind"`
	Name types.String `tfsdk:"name"`
}

type QuayNotificationConfig struct {
	Target QuayNotificationTarget `tfsdk:"target"`
}

type RepoPushEvent struct{}

type BuildEvent struct {
	RefRegex types.String `tfsdk:"ref_regex"`
}

type VulnerabilityFoundEvent struct {
	Level types.String `tfsdk:"level"`
}

type RepoMirrorSyncStartedEvent struct{}
type RepoMirrorSyncSuccessEvent struct{}
type RepoMirrorSyncFailedEvent struct{}

type RepoImageExpiryEvent struct {
	Days types.Int64 `tfsdk:"days"`
}

func NewRepositoryNotificationResource() resource.Resource {
	return &RepositoryNotificationResource{}
}

func (r *RepositoryNotificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_repository_notification"
}

// buildEventAttributes returns the attribute set shared by all build event blocks.
func buildEventAttributes(requiresReplace []planmodifier.String) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"ref_regex": schema.StringAttribute{
			Optional:      true,
			Description:   "A regular expression that Git refs must match to trigger the notification.",
			PlanModifiers: requiresReplace,
		},
	}
}

func (r *RepositoryNotificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	requiresReplaceObj := []planmodifier.Object{objectplanmodifier.RequiresReplace()}
	requiresReplaceInt := []planmodifier.Int64{int64planmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "Manages a notification on a Quay repository. " +
			"Because the Quay API has no update endpoint for notifications, any change to notification " +
			"attributes will cause the existing notification to be deleted and a new one created. " +
			"Exactly one method block (webhook, slack, email, or quay_notification) " +
			"must be provided. Exactly one event block must be provided; event-specific configuration " +
			"(such as the vulnerability severity level or a build ref regex) lives inside that block.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The resource identifier (repository/uuid).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Required:      true,
				Description:   "The namespace (organization or username) that owns the repository.",
				PlanModifiers: requiresReplaceStr,
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "The name of the repository.",
				PlanModifiers: requiresReplaceStr,
			},
			"uuid": schema.StringAttribute{
				Computed:    true,
				Description: "The server-assigned UUID of the notification.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "A human-readable title for the notification.",
				PlanModifiers: append(requiresReplaceStr, stringplanmodifier.UseStateForUnknown()),
			},
			"number_of_failures": schema.Int64Attribute{
				Computed:    true,
				Description: "The number of consecutive delivery failures.",
			},

			"webhook": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications via an HTTP POST to a URL.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						Required:      true,
						Sensitive:     true,
						Description:   "The URL to POST the notification payload to. Marked sensitive because webhook URLs frequently embed authentication tokens.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"slack": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications to a Slack channel via an Incoming Webhook URL.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"url": schema.StringAttribute{
						Required:      true,
						Sensitive:     true,
						Description:   "The Slack Incoming Webhook URL. Marked sensitive because the URL itself is the credential.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"email": schema.SingleNestedAttribute{
				Optional: true,
				Description: "Deliver notifications by email. " +
					"The address must already be authorized and confirmed for the repository in the Quay UI " +
					"before this resource is created; Quay rejects unverified addresses with 400 " +
					"\"not authorized to receive notifications\" and the provider cannot run the confirmation flow.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"email": schema.StringAttribute{
						Required:      true,
						Description:   "The recipient email address.",
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"quay_notification": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Deliver notifications as in-app alerts in the Quay web UI to a specific user or team. The recipient must be a member of the repository's namespace.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"target": schema.SingleNestedAttribute{
						Required:      true,
						Description:   "The Quay user or team to send the in-app notification to.",
						PlanModifiers: requiresReplaceObj,
						Attributes: map[string]schema.Attribute{
							"kind": schema.StringAttribute{
								Required:    true,
								Description: `The kind of target: "user" to notify an individual Quay account, or "team" to notify all members of an organization team.`,
								Validators: []validator.String{
									stringvalidator.OneOf("user", "team"),
								},
								PlanModifiers: requiresReplaceStr,
							},
							"name": schema.StringAttribute{
								Required:      true,
								Description:   "The Quay username or team name to notify.",
								PlanModifiers: requiresReplaceStr,
							},
						},
					},
				},
			},

			"repo_push": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger on every image push to the repository.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"build_queued": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build is queued.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_start": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build starts.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_success": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build succeeds.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_failure": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build fails.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"build_cancelled": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a build is cancelled.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    buildEventAttributes(requiresReplaceStr),
			},
			"vulnerability_found": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a vulnerability is found.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"level": schema.StringAttribute{
						Optional: true,
						Description: "The minimum vulnerability severity level that triggers the notification. " +
							"One of: Critical, High, Medium, Low, Negligible, Unknown. " +
							"When omitted, the notification fires for any severity. " +
							"These names match the severity labels shown in the Quay UI.",
						Validators: []validator.String{
							stringvalidator.OneOf("Critical", "High", "Medium", "Low", "Negligible", "Unknown"),
						},
						PlanModifiers: requiresReplaceStr,
					},
				},
			},
			"repo_mirror_sync_started": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a repository mirror sync starts.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"repo_mirror_sync_success": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a repository mirror sync succeeds.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"repo_mirror_sync_failed": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when a repository mirror sync fails.",
				PlanModifiers: requiresReplaceObj,
				Attributes:    map[string]schema.Attribute{},
			},
			"repo_image_expiry": schema.SingleNestedAttribute{
				Optional:      true,
				Description:   "Trigger when an image in the repository is due to expire.",
				PlanModifiers: requiresReplaceObj,
				Attributes: map[string]schema.Attribute{
					"days": schema.Int64Attribute{
						Required:      true,
						Description:   "Trigger the notification when the image will expire within this many days.",
						PlanModifiers: requiresReplaceInt,
					},
				},
			},
		},
	}
}

// ConfigValidators enforces that exactly one method block and one event block are present.
func (r *RepositoryNotificationResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("webhook"),
			path.MatchRoot("slack"),
			path.MatchRoot("email"),
			path.MatchRoot("quay_notification"),
		),
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("repo_push"),
			path.MatchRoot("build_queued"),
			path.MatchRoot("build_start"),
			path.MatchRoot("build_success"),
			path.MatchRoot("build_failure"),
			path.MatchRoot("build_cancelled"),
			path.MatchRoot("vulnerability_found"),
			path.MatchRoot("repo_mirror_sync_started"),
			path.MatchRoot("repo_mirror_sync_success"),
			path.MatchRoot("repo_mirror_sync_failed"),
			path.MatchRoot("repo_image_expiry"),
		),
	}
}

func (r *RepositoryNotificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

func (r *RepositoryNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RepositoryNotificationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	method, methodConfig := plan.methodAndConfig()
	event, eventConfig := plan.eventAndConfig()

	repoPath := plan.Namespace.ValueString() + "/" + plan.Name.ValueString()
	notif, err := r.client.CreateRepoNotification(ctx, repoPath, client.CreateNotificationRequest{
		Event:       event,
		Method:      method,
		Config:      methodConfig,
		EventConfig: eventConfig,
		Title:       plan.Title.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Repository Notification", err.Error())
		return
	}

	resp.Diagnostics.Append(r.mapToState(notif, repoPath, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *RepositoryNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RepositoryNotificationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repoPath := state.Namespace.ValueString() + "/" + state.Name.ValueString()
	notif, err := r.client.GetRepoNotification(ctx, repoPath, state.UUID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Repository Notification", err.Error())
		return
	}

	resp.Diagnostics.Append(r.mapToState(notif, repoPath, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *RepositoryNotificationResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All attributes carry RequiresReplace, so the framework should never call Update.
	resp.Diagnostics.AddError(
		"Unexpected Update",
		"The quay_repository_notification resource does not support in-place updates. "+
			"This is a bug in the provider — please report it at "+
			"https://github.com/quay/terraform-provider-quay/issues.",
	)
}

func (r *RepositoryNotificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RepositoryNotificationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	repoPath := state.Namespace.ValueString() + "/" + state.Name.ValueString()
	err := r.client.DeleteRepoNotification(ctx, repoPath, state.UUID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error Deleting Repository Notification", err.Error())
	}
}

func (r *RepositoryNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, diags := splitImportID(resp, req.ID, 3, "my-org/my-repo/32bb0ef6-ad69-4ccb-b64e-ddbc8bfd444d")
	if diags.HasError() {
		return
	}
	namespace := parts[0]
	repoName := parts[1]
	uuid := parts[2]
	setAttrs(ctx, resp, map[string]string{
		// id is derived from the parsed parts so it is always canonical and
		// matches what mapToState produces during Create and Read.
		"id":        namespace + "/" + repoName + "/" + uuid,
		"namespace": namespace,
		"name":      repoName,
		"uuid":      uuid,
	})
}

// methodAndConfig returns the API method string and config map for the active method block.
func (m *RepositoryNotificationResourceModel) methodAndConfig() (string, map[string]any) {
	switch {
	case m.Webhook != nil:
		return "webhook", map[string]any{"url": m.Webhook.URL.ValueString()}
	case m.Slack != nil:
		return "slack", map[string]any{"url": m.Slack.URL.ValueString()}
	case m.Email != nil:
		return "email", map[string]any{"email": m.Email.Email.ValueString()}
	case m.QuayNotification != nil:
		return "quay_notification", map[string]any{
			"target": map[string]any{
				"kind": m.QuayNotification.Target.Kind.ValueString(),
				"name": m.QuayNotification.Target.Name.ValueString(),
			},
		}
	default:
		// Unreachable: ConfigValidators ensures exactly one method block is set.
		return "", map[string]any{}
	}
}

// eventAndConfig returns the API event string and event_config map for the active event block.
func (m *RepositoryNotificationResourceModel) eventAndConfig() (string, map[string]any) {
	switch {
	case m.RepoPush != nil:
		return "repo_push", map[string]any{}
	case m.BuildQueued != nil:
		return "build_queued", buildRefRegexConfig(m.BuildQueued.RefRegex)
	case m.BuildStart != nil:
		return "build_start", buildRefRegexConfig(m.BuildStart.RefRegex)
	case m.BuildSuccess != nil:
		return "build_success", buildRefRegexConfig(m.BuildSuccess.RefRegex)
	case m.BuildFailure != nil:
		return "build_failure", buildRefRegexConfig(m.BuildFailure.RefRegex)
	case m.BuildCancelled != nil:
		return "build_cancelled", buildRefRegexConfig(m.BuildCancelled.RefRegex)
	case m.VulnerabilityFound != nil:
		if m.VulnerabilityFound.Level.IsNull() {
			return "vulnerability_found", map[string]any{}
		}
		idx, _ := client.VulnerabilityLevelIndexFromString(m.VulnerabilityFound.Level.ValueString())
		return "vulnerability_found", map[string]any{"level": idx}
	case m.RepoImageExpiry != nil:
		return "repo_image_expiry", map[string]any{"days": m.RepoImageExpiry.Days.ValueInt64()}
	case m.RepoMirrorSyncStarted != nil:
		return "repo_mirror_sync_started", map[string]any{}
	case m.RepoMirrorSyncSuccess != nil:
		return "repo_mirror_sync_success", map[string]any{}
	case m.RepoMirrorSyncFailed != nil:
		return "repo_mirror_sync_failed", map[string]any{}
	default:
		// Unreachable: ConfigValidators ensures exactly one event block is set.
		return "", map[string]any{}
	}
}

// buildRefRegexConfig returns an event_config map for build events, omitting ref-regex when unset.
func buildRefRegexConfig(refRegex types.String) map[string]any {
	cfg := map[string]any{}
	if !refRegex.IsNull() && !refRegex.IsUnknown() {
		cfg["ref-regex"] = refRegex.ValueString()
	}
	return cfg
}

// mapToState populates the model from a Notification API response.
func (r *RepositoryNotificationResource) mapToState(notif *client.Notification, repository string, state *RepositoryNotificationResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	parts := strings.SplitN(repository, "/", 2)
	state.Namespace = types.StringValue(parts[0])
	if len(parts) == 2 {
		state.Name = types.StringValue(parts[1])
	}
	state.UUID = types.StringValue(notif.UUID)
	state.Title = types.StringValue(notif.Title)
	state.NumberOfFailures = types.Int64Value(int64(notif.NumberOfFailures))
	state.ID = types.StringValue(fmt.Sprintf("%s/%s", repository, notif.UUID))

	state.Webhook = nil
	state.Slack = nil
	state.Email = nil
	state.QuayNotification = nil

	switch notif.Method {
	case "webhook":
		state.Webhook = &WebhookConfig{URL: types.StringValue(stringFromConfig(notif.Config, "url"))}
	case "slack":
		state.Slack = &SlackConfig{URL: types.StringValue(stringFromConfig(notif.Config, "url"))}
	case "email":
		state.Email = &EmailConfig{Email: types.StringValue(stringFromConfig(notif.Config, "email"))}
	case "quay_notification":
		target, _ := notif.Config["target"].(map[string]any)
		kind := ""
		name := ""
		if target != nil {
			kind = fmt.Sprintf("%v", target["kind"])
			name = fmt.Sprintf("%v", target["name"])
		}
		state.QuayNotification = &QuayNotificationConfig{
			Target: QuayNotificationTarget{
				Kind: types.StringValue(kind),
				Name: types.StringValue(name),
			},
		}
	default:
		diags.AddError("Unknown Notification Method",
			fmt.Sprintf("The Quay API returned an unrecognised notification method %q. "+
				"Please report this as a provider bug.", notif.Method))
		return diags
	}

	state.RepoPush = nil
	state.BuildQueued = nil
	state.BuildStart = nil
	state.BuildSuccess = nil
	state.BuildFailure = nil
	state.BuildCancelled = nil
	state.VulnerabilityFound = nil
	state.RepoMirrorSyncStarted = nil
	state.RepoMirrorSyncSuccess = nil
	state.RepoMirrorSyncFailed = nil
	state.RepoImageExpiry = nil

	switch notif.Event {
	case "repo_push":
		state.RepoPush = &RepoPushEvent{}
	case "build_queued":
		state.BuildQueued = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_start":
		state.BuildStart = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_success":
		state.BuildSuccess = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_failure":
		state.BuildFailure = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "build_cancelled":
		state.BuildCancelled = &BuildEvent{RefRegex: refRegexFromEventConfig(notif.EventConfig)}
	case "vulnerability_found":
		var level types.String
		if levelStr, ok := notif.EventConfig.VulnerabilityLevelString(); ok {
			level = types.StringValue(levelStr)
		} else {
			// Quay omits the level when "any severity" is selected.
			level = types.StringNull()
		}
		state.VulnerabilityFound = &VulnerabilityFoundEvent{Level: level}
	case "repo_mirror_sync_started":
		state.RepoMirrorSyncStarted = &RepoMirrorSyncStartedEvent{}
	case "repo_mirror_sync_success":
		state.RepoMirrorSyncSuccess = &RepoMirrorSyncSuccessEvent{}
	case "repo_mirror_sync_failed":
		state.RepoMirrorSyncFailed = &RepoMirrorSyncFailedEvent{}
	case "repo_image_expiry":
		var days types.Int64
		if v, ok := notif.EventConfig["days"]; ok && v != nil {
			days = types.Int64Value(int64(v.(float64)))
		} else {
			days = types.Int64Null()
		}
		state.RepoImageExpiry = &RepoImageExpiryEvent{Days: days}
	default:
		diags.AddError("Unknown Notification Event",
			fmt.Sprintf("The Quay API returned an unrecognised notification event %q. "+
				"Please report this as a provider bug.", notif.Event))
		return diags
	}

	return diags
}

// refRegexFromEventConfig extracts ref-regex from a build event config, or returns null when absent.
func refRegexFromEventConfig(eventConfig client.EventConfig) types.String {
	if v, ok := eventConfig["ref-regex"]; ok {
		return types.StringValue(fmt.Sprintf("%v", v))
	}
	return types.StringNull()
}

// stringFromConfig extracts a string value from a map[string]any, returning "" when absent.
func stringFromConfig(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
