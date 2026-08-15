terraform { required_version = ">= 1.6" }

variable "scenario" { type = string }
variable "phase" {
  type    = string
  default = "baseline"
}

locals { changed = var.phase == "changed" }

resource "terraform_data" "create" {
  count = var.scenario == "create" && local.changed ? 1 : 0
  input = "created-in-changed-phase"
}

resource "terraform_data" "destroy" {
  count = var.scenario == "destroy" && !local.changed ? 1 : 0
  input = "present-in-baseline"
}

resource "terraform_data" "update" {
  count = var.scenario == "update" ? 1 : 0
  input = local.changed ? "v2" : "v1"
}

resource "terraform_data" "replace" {
  count            = var.scenario == "replace" ? 1 : 0
  triggers_replace = local.changed ? "gen2" : "gen1"
  input            = "constant"
}

resource "terraform_data" "noop" {
  count = var.scenario == "noop" ? 1 : 0
  input = "never-changes"
}
