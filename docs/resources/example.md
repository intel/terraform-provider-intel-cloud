---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "intelcloud_instance Resource - intelcloud"
subcategory: ""
description: |-
  Virtual Machine Instance
---

# scaffolding_example (Resource)

Example resource

## Example Usage

```terraform
data "intelcloud_machine_images" "image" {
  most_recent = true
  filters = [
    {
      name   = "name"
      values = ["ubuntu-2204-jammy"]
    }
  ]
}

resource "intelcloud_instance" "example" {
  name = "tf-demo-instance"
  spec = {
    instance_type        = "vm-spr-sml"
    machine_image        = data.intelcloud_machine_images.image.result.name
    ssh_public_key_names = ["shrimac"]
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

### Read-Only

- `id` (String) Instance identifier
