include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules//app"
}

inputs = {
  name           = "prod-db"
  instance_count = 1
  engine_version = "14.7"
}
