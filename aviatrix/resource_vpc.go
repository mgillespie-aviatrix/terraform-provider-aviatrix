package aviatrix

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/terraform-providers/terraform-provider-aviatrix/goaviatrix"
)

func resourceVpc() *schema.Resource {
	return &schema.Resource{
		Create: resourceVpcCreate,
		Read:   resourceVpcRead,
		Update: resourceVpcUpdate,
		Delete: resourceVpcDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"cloud_type": {
				Type:        schema.TypeInt,
				Required:    true,
				Description: "Type of cloud service provider.",
			},
			"account_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Account name. This account will be used to create an Aviatrix VPC.",
			},
			"region": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Region where this gateway will be launched.",
			},
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Name of the VPC to be created.",
			},
			"cidr": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Subnet of the VPC to be created.",
			},
			"aviatrix_transit_vpc": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Specify the VPC as Aviatrix Transit VPC or not.",
			},
			"vpc_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "ID of the VPC created.",
			},
			"subnets": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "Subnets of the VPC to be created.",
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"cidr": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Subnet cidr.",
						},
						"name": {
							Type:        schema.TypeString,
							Optional:    true,
							Description: "Subnet name.",
						},
					},
				},
			},
		},
	}
}

func resourceVpcCreate(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*goaviatrix.Client)
	vpc := &goaviatrix.Vpc{
		CloudType:   d.Get("cloud_type").(int),
		AccountName: d.Get("account_name").(string),
		Region:      d.Get("region").(string),
		Name:        d.Get("name").(string),
		Cidr:        d.Get("cidr").(string),
	}

	if vpc.Region == "" {
		return fmt.Errorf("region can not be empty")
	}
	aviatrixTransitVpc := d.Get("aviatrix_transit_vpc").(bool)
	if aviatrixTransitVpc {
		vpc.AviatrixTransitVpc = "yes"
	} else {
		vpc.AviatrixTransitVpc = "no"
	}

	if vpc.AviatrixTransitVpc == "yes" {
		log.Printf("[INFO] Creating a new Aviatrix Transit VPC: %#v", vpc)
	} else {
		log.Printf("[INFO] Creating a new VPC: %#v", vpc)
	}

	err := client.CreateVpc(vpc)
	if err != nil {
		if vpc.AviatrixTransitVpc == "yes" {
			return fmt.Errorf("failed to create a new Aviatrix Transit VPC: %s", err)
		}
		return fmt.Errorf("failed to create a new VPC: %s", err)
	}

	d.SetId(vpc.Name)

	return resourceVpcRead(d, meta)
}

func resourceVpcRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*goaviatrix.Client)

	vpcName := d.Get("name").(string)
	if vpcName == "" {
		id := d.Id()
		log.Printf("[DEBUG] Looks like an import, no vpc names received. Import Id is %s", id)
		d.Set("name", id)
		d.SetId(id)
	}

	vpc := &goaviatrix.Vpc{
		Name: d.Get("name").(string),
	}
	vC, err := client.GetVpc(vpc)
	if err != nil {
		if err == goaviatrix.ErrNotFound {
			d.SetId("")
			return nil
		}
		return fmt.Errorf("couldn't find VPC: %s", err)
	}

	log.Printf("[INFO] Found VPC: %#v", vpc)

	d.Set("cloud_type", vC.CloudType)
	d.Set("account_name", vC.AccountName)
	d.Set("region", vC.Region)
	d.Set("name", vC.Name)
	d.Set("cidr", vC.Cidr)
	if vC.AviatrixTransitVpc == "yes" {
		d.Set("aviatrix_transit_vpc", true)
	} else {
		d.Set("aviatrix_transit_vpc", false)
	}
	d.Set("vpc_id", vC.VpcID)

	var subnetList []map[string]string
	for _, subnet := range vC.Subnets {
		sub := make(map[string]string)
		sub["cidr"] = subnet.Cidr
		sub["name"] = subnet.Name

		subnetList = append(subnetList, sub)
	}
	d.Set("subnets", subnetList)

	return nil
}

func resourceVpcUpdate(d *schema.ResourceData, meta interface{}) error {
	return fmt.Errorf("aviatrix VPC does not support update - delete and create new one")
}

func resourceVpcDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*goaviatrix.Client)
	vpc := &goaviatrix.Vpc{
		AccountName: d.Get("account_name").(string),
		Name:        d.Get("name").(string),
	}

	log.Printf("[INFO] Deleting VPC: %#v", vpc)

	err := client.DeleteVpc(vpc)
	if err != nil {
		return fmt.Errorf("failed to delete VPC: %s", err)
	}

	return nil
}
