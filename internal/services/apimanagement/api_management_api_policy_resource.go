package apimanagement

import (
	"fmt"
	"html"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/apimanagement/mgmt/2021-08-01/apimanagement"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/apimanagement/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/apimanagement/schemaz"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceApiManagementApiPolicy() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceApiManagementAPIPolicyCreateUpdate,
		Read:   resourceApiManagementAPIPolicyRead,
		Update: resourceApiManagementAPIPolicyCreateUpdate,
		Delete: resourceApiManagementAPIPolicyDelete,
		Importer: pluginsdk.ImporterValidatingResourceId(func(id string) error {
			_, err := parse.ApiPolicyID(id)
			return err
		}),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"resource_group_name": azure.SchemaResourceGroupName(),

			"api_management_name": schemaz.SchemaApiManagementName(),

			"api_name": schemaz.SchemaApiManagementApiName(),

			"xml_content": {
				Type:             pluginsdk.TypeString,
				Optional:         true,
				Computed:         true,
				ConflictsWith:    []string{"xml_link"},
				DiffSuppressFunc: XmlWithDotNetInterpolationsDiffSuppress,
			},

			"xml_link": {
				Type:          pluginsdk.TypeString,
				Optional:      true,
				ConflictsWith: []string{"xml_content"},
			},
		},
	}
}

func resourceApiManagementAPIPolicyCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.ApiPoliciesClient
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	resourceGroup := d.Get("resource_group_name").(string)
	serviceName := d.Get("api_management_name").(string)
	apiName := d.Get("api_name").(string)

	if d.IsNewResource() {
		existing, err := client.Get(ctx, resourceGroup, serviceName, apiName, apimanagement.PolicyExportFormatXML)
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing API Policy (API Management Service %q / API %q / Resource Group %q): %s", serviceName, apiName, resourceGroup, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_api_management_api_policy", *existing.ID)
		}
	}

	parameters := apimanagement.PolicyContract{}

	xmlContent := d.Get("xml_content").(string)
	xmlLink := d.Get("xml_link").(string)

	if xmlLink != "" {
		parameters.PolicyContractProperties = &apimanagement.PolicyContractProperties{
			Format: apimanagement.PolicyContentFormatRawxmlLink,
			Value:  utils.String(xmlLink),
		}
	} else if xmlContent != "" {
		// this is intentionally an else-if since `xml_content` is computed

		// clear out any existing value for xml_link
		if !d.IsNewResource() {
			d.Set("xml_link", "")
		}

		parameters.PolicyContractProperties = &apimanagement.PolicyContractProperties{
			Format: apimanagement.PolicyContentFormatRawxml,
			Value:  utils.String(xmlContent),
		}
	}

	if parameters.PolicyContractProperties == nil {
		return fmt.Errorf("Either `xml_content` or `xml_link` must be set")
	}

	if _, err := client.CreateOrUpdate(ctx, resourceGroup, serviceName, apiName, parameters, ""); err != nil {
		return fmt.Errorf("creating or updating API Policy (Resource Group %q / API Management Service %q / API %q): %+v", resourceGroup, serviceName, apiName, err)
	}

	resp, err := client.Get(ctx, resourceGroup, serviceName, apiName, apimanagement.PolicyExportFormatXML)
	if err != nil {
		return fmt.Errorf("retrieving API Policy (Resource Group %q / API Management Service %q / API %q): %+v", resourceGroup, serviceName, apiName, err)
	}
	if resp.ID == nil {
		return fmt.Errorf("Cannot read ID for API Policy (Resource Group %q / API Management Service %q / API %q): %+v", resourceGroup, serviceName, apiName, err)
	}
	d.SetId(*resp.ID)

	return resourceApiManagementAPIPolicyRead(d, meta)
}

func resourceApiManagementAPIPolicyRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.ApiPoliciesClient
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.ApiPolicyID(d.Id())
	if err != nil {
		return err
	}

	resp, err := client.Get(ctx, id.ResourceGroup, id.ServiceName, id.ApiName, apimanagement.PolicyExportFormatXML)
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			log.Printf("[DEBUG] %s was not found - removing from state!", *id)
			d.SetId("")
			return nil
		}

		return fmt.Errorf("making Read request for %s: %+v", *id, err)
	}

	d.Set("resource_group_name", id.ResourceGroup)
	d.Set("api_management_name", id.ServiceName)
	d.Set("api_name", id.ApiName)

	if properties := resp.PolicyContractProperties; properties != nil {
		policyContent := ""
		if pc := properties.Value; pc != nil {
			policyContent = html.UnescapeString(*pc)
		}

		// when you submit an `xml_link` to the API, the API downloads this link and stores it as `xml_content`
		// as such there is no way to set `xml_link` and we'll let Terraform handle it
		d.Set("xml_content", policyContent)
	}

	return nil
}

func resourceApiManagementAPIPolicyDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).ApiManagement.ApiPoliciesClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.ApiPolicyID(d.Id())
	if err != nil {
		return err
	}

	if resp, err := client.Delete(ctx, id.ResourceGroup, id.ServiceName, id.ApiName, ""); err != nil {
		if !utils.ResponseWasNotFound(resp) {
			return fmt.Errorf("deleting %s: %+v", *id, err)
		}
	}

	return nil
}
