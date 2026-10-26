variable "program_group" {
  description = "Full path of the existing group the layout is built under"
  type        = string
}

variable "scrums" {
  description = "Scrum subgroup -> its repos. An entry is a repo name (a gated code repo) or { name, type } with type code or experiments."
  type        = map(list(any))
}

variable "root_projects" {
  description = "Repos that live directly in the program group"
  type        = list(string)
  default     = []
}

variable "labels" {
  description = "Group label name -> color"
  type        = map(string)
  default     = {}
}

variable "members" {
  description = "Username -> { scrum, role }; every member also gets reporter on the program group"
  type        = map(object({ scrum = string, role = string }))
  default     = {}
}

variable "recurring_by_type" {
  description = "Recurring issues seeded at repository creation, by repository type"
  type        = map(list(object({ title = string, description = string })))
  default     = { code = [], experiments = [] }
}

variable "extra_recurring" {
  description = "Recurring issues seeded at repository creation, by repo key (scrum/name or a root project name)"
  type        = map(list(object({ title = string, description = string })))
  default     = {}
}

variable "service_accounts_enabled" {
  description = "Create service accounts; requires Premium or Ultimate"
  type        = bool
  default     = false
}

variable "ci_config_project" {
  description = "Project path holding ci/<type>.gitlab-ci.yml for every repo; empty sets no ci_config_path"
  type        = string
  default     = ""
}

variable "require_pipeline" {
  description = "Require a passing pipeline before code repos merge"
  type        = bool
  default     = true
}

variable "protect_projects" {
  description = "true: projects carry prevent_destroy (production). false: projects carry no prevent_destroy; mage regression:gitlab removes them with their rig group."
  type        = bool
}
