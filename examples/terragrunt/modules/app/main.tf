terraform {
  required_version = ">= 1.6"
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

variable "name" {
  type        = string
  description = "Name prefix for this unit's resources."
}

variable "instance_count" {
  type        = number
  default     = 1
  description = "How many random_pet 'instances' to create."
}

variable "engine_version" {
  type        = string
  default     = "14.7"
  description = "Bumping this changes random_id.release's keeper, forcing a replacement (-/+) — mirrors a DB engine upgrade."
}

# Plain creates/destroys as instance_count changes.
resource "random_pet" "instance" {
  count  = var.instance_count
  prefix = var.name
}

# Keyed on engine_version, so changing it forces this resource to be replaced.
resource "random_id" "release" {
  byte_length = 8
  keepers = {
    engine_version = var.engine_version
  }
}

# A sensitive attribute, so gruntcmt renders "(sensitive value)".
resource "random_password" "secret" {
  length  = 20
  special = true
}

output "pets" {
  value = random_pet.instance[*].id
}
