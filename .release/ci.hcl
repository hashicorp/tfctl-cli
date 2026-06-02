schema = "2"

project "tfctl-cli" {
  team = "tfctl-cli"
  slack {
    notification_channel = "C09KY0KNNFQ"
  }
  github {
    organization     = "hashicorp"
    repository       = "tfctl-cli"
    release_branches = ["main", "release/**"]
  }
}

event "merge" {
}

event "build" {
  action "build" {
    organization = "hashicorp"
    repository   = "tfctl-cli"
    workflow     = "build"
    depends      = null
    config       = ""
  }
  depends = ["merge"]
}

event "prepare" {
  action "prepare" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "prepare"
    depends      = ["build"]
    config       = ""
  }
  depends = ["build"]
  notification {
    on = "fail"
  }
}

event "trigger-staging" {
}

event "promote-staging" {
  action "promote-staging" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-staging"
    depends      = null
    config       = "release-metadata.hcl"
  }
  depends = ["trigger-staging"]
  notification {
    on = "always"
  }
}

event "trigger-production" {
}

event "promote-production" {
  action "promote-production" {
    organization = "hashicorp"
    repository   = "crt-workflows-common"
    workflow     = "promote-production"
    depends      = null
    config       = ""
  }
  depends = ["trigger-production"]
  notification {
    on = "always"
  }
}