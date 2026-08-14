# Root terragrunt config, included by every unit via find_in_parent_folders().
# It lives ABOVE live/ so `run --all` (invoked inside live/) does not treat it as
# a unit. Kept intentionally small: the random provider needs no configuration and
# state is local, so there is nothing cloud-specific to share here.

# Common inputs available to every unit (units may override).
inputs = {
  engine_version = "14.7"
}
