variable "group" {
  description = "Full path of the existing group rig subgroups are created under"
  type        = string
}

variable "run_id" {
  description = "Unique id of this run; the subgroup is rig-<run_id>"
  type        = string
}

variable "po_repos" {
  description = "Number of Product Owner repositories"
  type        = number
  default     = 2
}

variable "reports" {
  description = "Create the reports repository"
  type        = bool
  default     = true
}

variable "service_accounts_enabled" {
  description = "Create service accounts; requires Premium or Ultimate"
  type        = bool
  default     = false
}
