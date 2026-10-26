locals {
  repos = merge(
    { for p in var.root_projects : p => { namespace = "root", name = p, type = "code" } },
    merge([for scrum, entries in var.scrums : {
      for e in entries : "${scrum}/${try(e.name, e)}" => {
        namespace = scrum
        name      = try(e.name, e)
        type      = try(e.type, "code")
      }
    }]...)
  )

  recurring_instances = merge([
    for key, repo in local.repos : {
      for issue in concat(
        try(var.recurring_by_type[repo.type], []),
        try(var.extra_recurring[key], [])
      ) : "${key}:${issue.title}" => { repo = key, title = issue.title, description = issue.description }
    }
  ]...)

  # scope is "program" or a scrum name; the membership role there is the
  # account's real limit.
  service_accounts = var.service_accounts_enabled ? merge(
    { track-goals = { scope = "program", role = "reporter" } },
    contains(keys(var.scrums), "test") ? { release = { scope = "test", role = "maintainer" } } : {},
    { for s in keys(var.scrums) : "${s}-agent" => { scope = s, role = "developer" } }
  ) : {}

  # One map for every reference, whichever resource holds the project.
  projects = merge(gitlab_project.protected, gitlab_project.disposable)

  project_settings = {
    for k, r in local.repos : k => {
      name             = r.name
      namespace_id     = r.namespace == "root" ? data.gitlab_group.program.group_id : gitlab_group.scrum[r.namespace].id
      require_pipeline = r.type == "code" && var.require_pipeline
      ci_config_path   = var.ci_config_project == "" ? null : "ci/${r.type}.gitlab-ci.yml@${var.ci_config_project}"
    }
  }
}

# The program group stays out of state so a destroy cannot take it with it.
data "gitlab_group" "program" {
  full_path = var.program_group
}

# GitLab creates group service accounts only on a top-level group.
data "gitlab_group" "root" {
  full_path = split("/", var.program_group)[0]
}

resource "gitlab_group" "scrum" {
  for_each = var.scrums

  name             = each.key
  path             = each.key
  parent_id        = data.gitlab_group.program.group_id
  visibility_level = "private"
}

data "gitlab_user" "member" {
  for_each = var.members

  username = each.key
}

# Baseline: reporter on the program group inherits down to every scrum.
resource "gitlab_group_membership" "program_read" {
  for_each = var.members

  group_id     = data.gitlab_group.program.group_id
  user_id      = data.gitlab_user.member[each.key].id
  access_level = "reporter"
}

# Working role in the home scrum, raising the inherited reporter there.
resource "gitlab_group_membership" "home_scrum" {
  for_each = var.members

  group_id     = gitlab_group.scrum[each.value.scrum].id
  user_id      = data.gitlab_user.member[each.key].id
  access_level = each.value.role
}

resource "gitlab_group_label" "program" {
  for_each = var.labels

  group       = data.gitlab_group.program.id
  name        = each.key
  color       = each.value
  description = "managed by terraform"
}

# Terraform does not accept a variable in a lifecycle block, so production
# projects and disposable projects are two resources selected by
# protect_projects. Project paths are pinned-citation targets in production.
resource "gitlab_project" "protected" {
  for_each = var.protect_projects ? local.repos : {}

  name                                  = local.project_settings[each.key].name
  namespace_id                          = local.project_settings[each.key].namespace_id
  visibility_level                      = "private"
  default_branch                        = "main"
  initialize_with_readme                = true
  only_allow_merge_if_pipeline_succeeds = local.project_settings[each.key].require_pipeline
  remove_source_branch_after_merge      = true
  ci_config_path                        = local.project_settings[each.key].ci_config_path

  lifecycle {
    prevent_destroy = true
  }
}

resource "gitlab_project" "disposable" {
  for_each = var.protect_projects ? {} : local.repos

  name                                  = local.project_settings[each.key].name
  namespace_id                          = local.project_settings[each.key].namespace_id
  visibility_level                      = "private"
  default_branch                        = "main"
  initialize_with_readme                = true
  only_allow_merge_if_pipeline_succeeds = local.project_settings[each.key].require_pipeline
  remove_source_branch_after_merge      = true
  ci_config_path                        = local.project_settings[each.key].ci_config_path
}

resource "gitlab_branch_protection" "main" {
  for_each = local.repos

  project            = local.projects[each.key].id
  branch             = "main"
  # tpm issue merge delivers by pushing a merge commit to main, so main
  # takes pushes from maintainers; experiments repositories from developers.
  push_access_level  = each.value.type == "experiments" ? "developer" : "maintainer"
  merge_access_level = each.value.type == "experiments" ? "developer" : "maintainer"
}

# Seeded once per repository; the tracker owns them from then on.
resource "gitlab_project_issue" "recurring" {
  for_each = local.recurring_instances

  project     = local.projects[each.value.repo].id
  title       = each.value.title
  description = each.value.description
  labels      = ["type::recurring"]

  depends_on = [gitlab_group_label.program]

  lifecycle {
    # Closing, editing, or re-creating an issue must not read as drift.
    ignore_changes = [state, description, labels, title]
  }
}

resource "gitlab_group_service_account" "bot" {
  for_each = local.service_accounts

  group    = data.gitlab_group.root.group_id
  name     = each.key
  username = "${replace(var.program_group, "/", "-")}-${each.key}"
}

resource "gitlab_group_membership" "bot" {
  for_each = local.service_accounts

  group_id     = each.value.scope == "program" ? data.gitlab_group.program.group_id : gitlab_group.scrum[each.value.scope].id
  user_id      = gitlab_group_service_account.bot[each.key].service_account_id
  access_level = each.value.role
}

resource "gitlab_group_service_account_access_token" "bot" {
  for_each = local.service_accounts

  group   = data.gitlab_group.root.group_id
  user_id = gitlab_group_service_account.bot[each.key].service_account_id
  name    = "${each.key}-ci"
  scopes  = ["api", "write_repository"]

  # Rotation happens on the first apply inside the window.
  rotation_configuration = {
    expiration_days    = 90
    rotate_before_days = 14
  }
}

resource "gitlab_group_variable" "bot_token" {
  for_each = local.service_accounts

  group     = each.value.scope == "program" ? data.gitlab_group.program.group_id : gitlab_group.scrum[each.value.scope].id
  key       = "${upper(replace(each.key, "-", "_"))}_TOKEN"
  value     = gitlab_group_service_account_access_token.bot[each.key].token
  masked    = true
  protected = true
}
