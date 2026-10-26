output "rig" {
  description = "The run's group and repositories, read by the integration targets"
  value = {
    run_id   = var.run_id
    group    = { id = gitlab_group.rig.id, full_path = gitlab_group.rig.full_path, web_url = gitlab_group.rig.web_url }
    projects = module.program.projects
  }
}
