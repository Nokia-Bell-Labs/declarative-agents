output "program_group" {
  description = "The program group the layout is built under"
  value       = { id = data.gitlab_group.program.group_id, full_path = data.gitlab_group.program.full_path }
}

output "projects" {
  description = "Repo key -> project id, path, and web URL"
  value = {
    for k, p in local.projects : k => {
      id                  = p.id
      path_with_namespace = p.path_with_namespace
      web_url             = p.web_url
    }
  }
}
