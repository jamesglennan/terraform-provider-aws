package aws

import (
	"fmt"
	"log"
	"math"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	events "github.com/aws/aws-sdk-go/service/cloudwatchevents"
	"github.com/hashicorp/aws-sdk-go-base/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/terraform-providers/terraform-provider-aws/aws/internal/keyvaluetags"
	tfevents "github.com/terraform-providers/terraform-provider-aws/aws/internal/service/cloudwatchevents"
	"github.com/terraform-providers/terraform-provider-aws/aws/internal/service/cloudwatchevents/finder"
)

func resourceAwsCloudWatchEventTarget() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsCloudWatchEventTargetCreate,
		Read:   resourceAwsCloudWatchEventTargetRead,
		Update: resourceAwsCloudWatchEventTargetUpdate,
		Delete: resourceAwsCloudWatchEventTargetDelete,

		Importer: &schema.ResourceImporter{
			State: resourceAwsCloudWatchEventTargetImport,
		},

		SchemaVersion: 1,
		StateUpgraders: []schema.StateUpgrader{
			{
				Type:    resourceAwsCloudWatchEventTargetV0().CoreConfigSchema().ImpliedType(),
				Upgrade: resourceAwsCloudWatchEventTargetStateUpgradeV0,
				Version: 0,
			},
		},

		Schema: map[string]*schema.Schema{
			"event_bus_name": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validateCloudWatchEventBusNameOrARN,
				Default:      tfevents.DefaultEventBusName,
			},

			"rule": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateCloudWatchEventBusNameOrARN,
			},

			"target_id": {
				Type:         schema.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validateCloudWatchEventTargetId,
			},

			"arn": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validateArn,
			},

			"input": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"input_path", "input_transformer"},
				// We could be normalizing the JSON here,
				// but for built-in targets input may not be JSON
			},

			"input_path": {
				Type:          schema.TypeString,
				Optional:      true,
				ConflictsWith: []string{"input", "input_transformer"},
			},

			"role_arn": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validateArn,
			},

			"run_command_targets": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 5,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"key": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringLenBetween(1, 128),
						},
						"values": {
							Type:     schema.TypeList,
							Required: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},

			"http_target": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"header_parameters": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"query_string_parameters": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"path_parameter_values": {
							Type:     schema.TypeSet,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
			},

			"ecs_target": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enable_ecs_managed_tags": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"enable_execute_command": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"group": {
							Type:         schema.TypeString,
							Optional:     true,
							ValidateFunc: validation.StringLenBetween(1, 255),
						},
						"launch_type": {
							Type:     schema.TypeString,
							Optional: true,
							Default:  events.LaunchTypeEc2,
							ValidateFunc: validation.Any(
								validation.StringIsEmpty,
								validation.StringInSlice(events.LaunchType_Values(), false),
							),
						},
						"network_configuration": {
							Type:     schema.TypeList,
							Optional: true,
							MaxItems: 1,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"security_groups": {
										Type:     schema.TypeSet,
										Optional: true,
										Elem:     &schema.Schema{Type: schema.TypeString},
									},
									"subnets": {
										Type:     schema.TypeSet,
										Required: true,
										Elem:     &schema.Schema{Type: schema.TypeString},
									},
									"assign_public_ip": {
										Type:     schema.TypeBool,
										Optional: true,
										Default:  false,
									},
								},
							},
						},
						"placement_constraint": {
							Type:     schema.TypeSet,
							Optional: true,
							MaxItems: 10,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"expression": {
										Type:     schema.TypeString,
										Optional: true,
									},
									"type": {
										Type:         schema.TypeString,
										Required:     true,
										ValidateFunc: validation.StringInSlice(events.PlacementConstraintType_Values(), false),
									},
								},
							},
						},
						"platform_version": {
							Type:         schema.TypeString,
							Optional:     true,
							ValidateFunc: validation.StringLenBetween(0, 1600),
						},
						"propagate_tags": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      events.PropagateTagsTaskDefinition,
							ValidateFunc: validation.StringInSlice(events.PropagateTags_Values(), false),
						},
						"tags": tagsSchema(),
						"task_count": {
							Type:         schema.TypeInt,
							Optional:     true,
							ValidateFunc: validation.IntBetween(1, math.MaxInt32),
							Default:      1,
						},
						"task_definition_arn": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validateArn,
						},
					},
				},
			},

			"batch_target": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"job_definition": {
							Type:     schema.TypeString,
							Required: true,
						},
						"job_name": {
							Type:     schema.TypeString,
							Required: true,
						},
						"array_size": {
							Type:         schema.TypeInt,
							Optional:     true,
							ValidateFunc: validation.IntBetween(2, 10000),
						},
						"job_attempts": {
							Type:         schema.TypeInt,
							Optional:     true,
							ValidateFunc: validation.IntBetween(1, 10),
						},
					},
				},
			},

			"kinesis_target": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"partition_key_path": {
							Type:         schema.TypeString,
							Optional:     true,
							ValidateFunc: validation.StringLenBetween(1, 256),
						},
					},
				},
			},

			"redshift_target": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"database": {
							Type:     schema.TypeString,
							Required: true,
						},
						"db_user": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"secrets_manager_arn": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"sql": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"statement_name": {
							Type:     schema.TypeString,
							Optional: true,
						},
						"with_event": {
							Type:     schema.TypeBool,
							Optional: true,
						},
					},
				},
			},

			"sqs_target": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"message_group_id": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},

			"input_transformer": {
				Type:          schema.TypeList,
				Optional:      true,
				MaxItems:      1,
				ConflictsWith: []string{"input", "input_path"},
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"input_paths": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
							ValidateFunc: validation.All(
								MapMaxItems(100),
								MapKeysDoNotMatch(regexp.MustCompile(`^AWS.*$`), "input_path must not start with \"AWS\""),
							),
						},
						"input_template": {
							Type:         schema.TypeString,
							Required:     true,
							ValidateFunc: validation.StringLenBetween(1, 8192),
						},
					},
				},
			},

			"retry_policy": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"maximum_event_age_in_seconds": {
							Type:         schema.TypeInt,
							Optional:     true,
							ValidateFunc: validation.IntAtLeast(60),
						},
						"maximum_retry_attempts": {
							Type:     schema.TypeInt,
							Optional: true,
						},
					},
				},
			},

			"dead_letter_config": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"arn": {
							Type:         schema.TypeString,
							Optional:     true,
							ValidateFunc: validateArn,
						},
					},
				},
			},
		},
	}
}

func resourceAwsCloudWatchEventTargetCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).cloudwatcheventsconn

	rule := d.Get("rule").(string)

	var targetID string
	if v, ok := d.GetOk("target_id"); ok {
		targetID = v.(string)
	} else {
		targetID = resource.UniqueId()
		d.Set("target_id", targetID)
	}
	var busName string
	if v, ok := d.GetOk("event_bus_name"); ok {
		busName = v.(string)
	}

	input := buildPutTargetInputStruct(d)

	log.Printf("[DEBUG] Creating CloudWatch Events Target: %s", input)
	out, err := conn.PutTargets(input)
	if err != nil {
		return fmt.Errorf("Creating CloudWatch Events Target failed: %w", err)
	}

	if len(out.FailedEntries) > 0 {
		return fmt.Errorf("Creating CloudWatch Events Target failed: %s", out.FailedEntries)
	}

	id := tfevents.TargetCreateID(busName, rule, targetID)
	d.SetId(id)

	log.Printf("[INFO] CloudWatch Events Target (%s) created", d.Id())

	return resourceAwsCloudWatchEventTargetRead(d, meta)
}

func resourceAwsCloudWatchEventTargetRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).cloudwatcheventsconn

	busName := d.Get("event_bus_name").(string)

	t, err := finder.Target(conn, busName, d.Get("rule").(string), d.Get("target_id").(string))
	if err != nil {
		if tfawserr.ErrCodeEquals(err, "ValidationException") ||
			tfawserr.ErrCodeEquals(err, events.ErrCodeResourceNotFoundException) ||
			regexp.MustCompile(" not found$").MatchString(err.Error()) {
			log.Printf("[WARN] CloudWatch Events Target (%s) not found, removing from state", d.Id())
			d.SetId("")
			return nil
		}
		return err
	}
	log.Printf("[DEBUG] Found Event Target: %s", t)

	d.Set("arn", t.Arn)
	d.Set("target_id", t.Id)
	d.Set("input", t.Input)
	d.Set("input_path", t.InputPath)
	d.Set("role_arn", t.RoleArn)
	d.Set("event_bus_name", busName)

	if t.RunCommandParameters != nil {
		if err := d.Set("run_command_targets", flattenAwsCloudWatchEventTargetRunParameters(t.RunCommandParameters)); err != nil {
			return fmt.Errorf("Error setting run_command_targets error: %w", err)
		}
	}

	if t.HttpParameters != nil {
		if err := d.Set("http_target", []interface{}{flattenAwsCloudWatchEventTargetHttpParameters(t.HttpParameters)}); err != nil {
			return fmt.Errorf("error setting http_target: %w", err)
		}
	} else {
		d.Set("http_target", nil)
	}

	if t.RedshiftDataParameters != nil {
		if err := d.Set("redshift_target", flattenAwsCloudWatchEventTargetRedshiftParameters(t.RedshiftDataParameters)); err != nil {
			return fmt.Errorf("Error setting ecs_target error: %w", err)
		}
	}

	if t.EcsParameters != nil {
		if err := d.Set("ecs_target", flattenAwsCloudWatchEventTargetEcsParameters(t.EcsParameters)); err != nil {
			return fmt.Errorf("Error setting ecs_target error: %w", err)
		}
	}

	if t.BatchParameters != nil {
		if err := d.Set("batch_target", flattenAwsCloudWatchEventTargetBatchParameters(t.BatchParameters)); err != nil {
			return fmt.Errorf("Error setting batch_target error: %w", err)
		}
	}

	if t.KinesisParameters != nil {
		if err := d.Set("kinesis_target", flattenAwsCloudWatchEventTargetKinesisParameters(t.KinesisParameters)); err != nil {
			return fmt.Errorf("Error setting kinesis_target error: %w", err)
		}
	}

	if t.SqsParameters != nil {
		if err := d.Set("sqs_target", flattenAwsCloudWatchEventTargetSqsParameters(t.SqsParameters)); err != nil {
			return fmt.Errorf("Error setting sqs_target error: %w", err)
		}
	}

	if t.InputTransformer != nil {
		if err := d.Set("input_transformer", flattenAwsCloudWatchInputTransformer(t.InputTransformer)); err != nil {
			return fmt.Errorf("Error setting input_transformer error: %w", err)
		}
	}

	if t.RetryPolicy != nil {
		if err := d.Set("retry_policy", flatternAwsCloudWatchEventTargetRetryPolicy(t.RetryPolicy)); err != nil {
			return fmt.Errorf("Error setting retry_policy error: #{err}")
		}
	}

	if t.DeadLetterConfig != nil {
		if err := d.Set("dead_letter_config", flatternAwsCloudWatchEventTargetDeadLetterConfig(t.DeadLetterConfig)); err != nil {
			return fmt.Errorf("Error setting dead_letter_config error: #{err}")
		}
	}

	return nil
}

func resourceAwsCloudWatchEventTargetUpdate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).cloudwatcheventsconn

	input := buildPutTargetInputStruct(d)

	log.Printf("[DEBUG] Updating CloudWatch Events Target: %s", input)
	_, err := conn.PutTargets(input)
	if err != nil {
		return fmt.Errorf("error updating CloudWatch Events Target (%s): %w", d.Id(), err)
	}

	return resourceAwsCloudWatchEventTargetRead(d, meta)
}

func resourceAwsCloudWatchEventTargetDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).cloudwatcheventsconn

	input := &events.RemoveTargetsInput{
		Ids:  []*string{aws.String(d.Get("target_id").(string))},
		Rule: aws.String(d.Get("rule").(string)),
	}

	if v, ok := d.GetOk("event_bus_name"); ok {
		input.EventBusName = aws.String(v.(string))
	}

	output, err := conn.RemoveTargets(input)
	if err != nil {
		if tfawserr.ErrCodeEquals(err, events.ErrCodeResourceNotFoundException) {
			return nil
		}
		return fmt.Errorf("error deleting CloudWatch Events Target (%s): %w", d.Id(), err)
	}

	if output != nil && len(output.FailedEntries) > 0 && output.FailedEntries[0] != nil {
		failedEntry := output.FailedEntries[0]
		return fmt.Errorf("error deleting CloudWatch Events Target (%s): failure entry: %s: %s", d.Id(), aws.StringValue(failedEntry.ErrorCode), aws.StringValue(failedEntry.ErrorMessage))
	}

	return nil
}

func buildPutTargetInputStruct(d *schema.ResourceData) *events.PutTargetsInput {
	e := &events.Target{
		Arn: aws.String(d.Get("arn").(string)),
		Id:  aws.String(d.Get("target_id").(string)),
	}

	if v, ok := d.GetOk("input"); ok {
		e.Input = aws.String(v.(string))
	}
	if v, ok := d.GetOk("input_path"); ok {
		e.InputPath = aws.String(v.(string))
	}

	if v, ok := d.GetOk("role_arn"); ok {
		e.RoleArn = aws.String(v.(string))
	}

	if v, ok := d.GetOk("run_command_targets"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.RunCommandParameters = expandAwsCloudWatchEventTargetRunParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("ecs_target"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.EcsParameters = expandAwsCloudWatchEventTargetEcsParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("redshift_target"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.RedshiftDataParameters = expandAwsCloudWatchEventTargetRedshiftParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("http_target"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.HttpParameters = expandAwsCloudWatchEventTargetHttpParameters(v.([]interface{})[0].(map[string]interface{}))
	}

	if v, ok := d.GetOk("batch_target"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.BatchParameters = expandAwsCloudWatchEventTargetBatchParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("kinesis_target"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.KinesisParameters = expandAwsCloudWatchEventTargetKinesisParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("sqs_target"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.SqsParameters = expandAwsCloudWatchEventTargetSqsParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("input_transformer"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.InputTransformer = expandAwsCloudWatchEventTransformerParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("retry_policy"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.RetryPolicy = expandAwsCloudWatchEventRetryPolicyParameters(v.([]interface{}))
	}

	if v, ok := d.GetOk("dead_letter_config"); ok && len(v.([]interface{})) > 0 && v.([]interface{})[0] != nil {
		e.DeadLetterConfig = expandAwsCloudWatchEventDeadLetterConfigParameters(v.([]interface{}))
	}

	input := events.PutTargetsInput{
		Rule:    aws.String(d.Get("rule").(string)),
		Targets: []*events.Target{e},
	}
	if v, ok := d.GetOk("event_bus_name"); ok {
		input.EventBusName = aws.String(v.(string))
	}

	return &input
}

func expandAwsCloudWatchEventTargetRunParameters(config []interface{}) *events.RunCommandParameters {
	commands := make([]*events.RunCommandTarget, 0)
	for _, c := range config {
		param := c.(map[string]interface{})
		command := &events.RunCommandTarget{
			Key:    aws.String(param["key"].(string)),
			Values: expandStringList(param["values"].([]interface{})),
		}
		commands = append(commands, command)
	}

	command := &events.RunCommandParameters{
		RunCommandTargets: commands,
	}

	return command
}

func expandAwsCloudWatchEventTargetRedshiftParameters(config []interface{}) *events.RedshiftDataParameters {
	redshiftParameters := &events.RedshiftDataParameters{}
	for _, c := range config {
		param := c.(map[string]interface{})

		redshiftParameters.Database = aws.String(param["database"].(string))
		redshiftParameters.Sql = aws.String(param["sql"].(string))

		if val, ok := param["with_event"].(bool); ok {
			redshiftParameters.WithEvent = aws.Bool(val)
		}

		if val, ok := param["statement_name"].(string); ok && val != "" {
			redshiftParameters.StatementName = aws.String(val)
		}

		if val, ok := param["secrets_manager_arn"].(string); ok && val != "" {
			redshiftParameters.SecretManagerArn = aws.String(val)
		}

		if val, ok := param["db_user"].(string); ok && val != "" {
			redshiftParameters.DbUser = aws.String(val)
		}
	}

	return redshiftParameters
}

func expandAwsCloudWatchEventTargetEcsParameters(config []interface{}) *events.EcsParameters {
	ecsParameters := &events.EcsParameters{}
	for _, c := range config {
		param := c.(map[string]interface{})
		tags := keyvaluetags.New(param["tags"].(map[string]interface{}))

		if val, ok := param["group"].(string); ok && val != "" {
			ecsParameters.Group = aws.String(val)
		}

		if val, ok := param["launch_type"].(string); ok && val != "" {
			ecsParameters.LaunchType = aws.String(val)
		}

		if val, ok := param["network_configuration"]; ok {
			ecsParameters.NetworkConfiguration = expandAwsCloudWatchEventTargetEcsParametersNetworkConfiguration(val.([]interface{}))
		}

		if val, ok := param["platform_version"].(string); ok && val != "" {
			ecsParameters.PlatformVersion = aws.String(val)
		}

		if v, ok := param["placement_constraint"].(*schema.Set); ok && v.Len() > 0 {
			ecsParameters.PlacementConstraints = expandAwsCloudWatchEventTargetPlacementConstraints(v.List())
		}

		if v, ok := param["propagate_tags"].(string); ok {
			ecsParameters.PropagateTags = aws.String(v)
		}

		if len(tags) > 0 {
			ecsParameters.Tags = tags.IgnoreAws().CloudwatcheventsTags()
		}

		ecsParameters.EnableExecuteCommand = aws.Bool(param["enable_execute_command"].(bool))
		ecsParameters.EnableECSManagedTags = aws.Bool(param["enable_ecs_managed_tags"].(bool))
		ecsParameters.TaskCount = aws.Int64(int64(param["task_count"].(int)))
		ecsParameters.TaskDefinitionArn = aws.String(param["task_definition_arn"].(string))
	}

	return ecsParameters
}

func expandAwsCloudWatchEventRetryPolicyParameters(rp []interface{}) *events.RetryPolicy {
	retryPolicy := &events.RetryPolicy{}

	for _, v := range rp {
		params := v.(map[string]interface{})

		if val, ok := params["maximum_event_age_in_seconds"].(int); ok {
			retryPolicy.MaximumEventAgeInSeconds = aws.Int64(int64(val))
		}

		if val, ok := params["maximum_retry_attempts"].(int); ok {
			retryPolicy.MaximumRetryAttempts = aws.Int64(int64(val))
		}
	}

	return retryPolicy
}

func expandAwsCloudWatchEventDeadLetterConfigParameters(dlp []interface{}) *events.DeadLetterConfig {
	deadLetterConfig := &events.DeadLetterConfig{}

	for _, v := range dlp {
		params := v.(map[string]interface{})

		if val, ok := params["arn"].(string); ok && val != "" {
			deadLetterConfig.Arn = aws.String(val)
		}
	}

	return deadLetterConfig
}

func expandAwsCloudWatchEventTargetEcsParametersNetworkConfiguration(nc []interface{}) *events.NetworkConfiguration {
	if len(nc) == 0 {
		return nil
	}
	awsVpcConfig := &events.AwsVpcConfiguration{}
	raw := nc[0].(map[string]interface{})
	if val, ok := raw["security_groups"]; ok {
		awsVpcConfig.SecurityGroups = expandStringSet(val.(*schema.Set))
	}
	awsVpcConfig.Subnets = expandStringSet(raw["subnets"].(*schema.Set))
	if val, ok := raw["assign_public_ip"].(bool); ok {
		awsVpcConfig.AssignPublicIp = aws.String(events.AssignPublicIpDisabled)
		if val {
			awsVpcConfig.AssignPublicIp = aws.String(events.AssignPublicIpEnabled)
		}
	}

	return &events.NetworkConfiguration{AwsvpcConfiguration: awsVpcConfig}
}

func expandAwsCloudWatchEventTargetBatchParameters(config []interface{}) *events.BatchParameters {
	batchParameters := &events.BatchParameters{}
	for _, c := range config {
		param := c.(map[string]interface{})
		batchParameters.JobDefinition = aws.String(param["job_definition"].(string))
		batchParameters.JobName = aws.String(param["job_name"].(string))
		if v, ok := param["array_size"].(int); ok && v > 1 && v <= 10000 {
			arrayProperties := &events.BatchArrayProperties{}
			arrayProperties.Size = aws.Int64(int64(v))
			batchParameters.ArrayProperties = arrayProperties
		}
		if v, ok := param["job_attempts"].(int); ok && v > 0 && v <= 10 {
			retryStrategy := &events.BatchRetryStrategy{}
			retryStrategy.Attempts = aws.Int64(int64(v))
			batchParameters.RetryStrategy = retryStrategy
		}
	}

	return batchParameters
}

func expandAwsCloudWatchEventTargetKinesisParameters(config []interface{}) *events.KinesisParameters {
	kinesisParameters := &events.KinesisParameters{}
	for _, c := range config {
		param := c.(map[string]interface{})
		if v, ok := param["partition_key_path"].(string); ok && v != "" {
			kinesisParameters.PartitionKeyPath = aws.String(v)
		}
	}

	return kinesisParameters
}

func expandAwsCloudWatchEventTargetSqsParameters(config []interface{}) *events.SqsParameters {
	sqsParameters := &events.SqsParameters{}
	for _, c := range config {
		param := c.(map[string]interface{})
		if v, ok := param["message_group_id"].(string); ok && v != "" {
			sqsParameters.MessageGroupId = aws.String(v)
		}
	}

	return sqsParameters
}

func expandAwsCloudWatchEventTargetHttpParameters(tfMap map[string]interface{}) *events.HttpParameters {
	if tfMap == nil {
		return nil
	}

	apiObject := &events.HttpParameters{}

	if v, ok := tfMap["header_parameters"].(map[string]interface{}); ok && len(v) > 0 {
		apiObject.HeaderParameters = expandStringMap(v)
	}

	if v, ok := tfMap["path_parameter_values"].(*schema.Set); ok && v.Len() > 0 {
		apiObject.PathParameterValues = expandStringSet(v)
	}

	if v, ok := tfMap["query_string_parameters"].(map[string]interface{}); ok && len(v) > 0 {
		apiObject.QueryStringParameters = expandStringMap(v)
	}

	return apiObject
}

func expandAwsCloudWatchEventTransformerParameters(config []interface{}) *events.InputTransformer {
	transformerParameters := &events.InputTransformer{}

	inputPathsMaps := map[string]*string{}

	for _, c := range config {
		param := c.(map[string]interface{})
		inputPaths := param["input_paths"].(map[string]interface{})

		for k, v := range inputPaths {
			inputPathsMaps[k] = aws.String(v.(string))
		}
		transformerParameters.InputTemplate = aws.String(param["input_template"].(string))
	}
	transformerParameters.InputPathsMap = inputPathsMaps

	return transformerParameters
}

func flattenAwsCloudWatchEventTargetRunParameters(runCommand *events.RunCommandParameters) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)

	for _, x := range runCommand.RunCommandTargets {
		config := make(map[string]interface{})

		config["key"] = aws.StringValue(x.Key)
		config["values"] = flattenStringList(x.Values)

		result = append(result, config)
	}

	return result
}

func flattenAwsCloudWatchEventTargetEcsParameters(ecsParameters *events.EcsParameters) []map[string]interface{} {
	config := make(map[string]interface{})
	if ecsParameters.Group != nil {
		config["group"] = aws.StringValue(ecsParameters.Group)
	}

	if ecsParameters.LaunchType != nil {
		config["launch_type"] = aws.StringValue(ecsParameters.LaunchType)
	}

	config["network_configuration"] = flattenAwsCloudWatchEventTargetEcsParametersNetworkConfiguration(ecsParameters.NetworkConfiguration)
	if ecsParameters.PlatformVersion != nil {
		config["platform_version"] = aws.StringValue(ecsParameters.PlatformVersion)
	}

	if ecsParameters.PropagateTags != nil {
		config["propagate_tags"] = aws.StringValue(ecsParameters.PropagateTags)
	}

	if ecsParameters.PlacementConstraints != nil {
		config["placement_constraint"] = flattenAwsCloudWatchEventTargetPlacementConstraints(ecsParameters.PlacementConstraints)
	}

	config["tags"] = keyvaluetags.CloudwatcheventsKeyValueTags(ecsParameters.Tags).IgnoreAws().Map()
	config["enable_execute_command"] = aws.BoolValue(ecsParameters.EnableExecuteCommand)
	config["enable_ecs_managed_tags"] = aws.BoolValue(ecsParameters.EnableECSManagedTags)
	config["task_count"] = aws.Int64Value(ecsParameters.TaskCount)
	config["task_definition_arn"] = aws.StringValue(ecsParameters.TaskDefinitionArn)
	result := []map[string]interface{}{config}
	return result
}

func flattenAwsCloudWatchEventTargetRedshiftParameters(redshiftParameters *events.RedshiftDataParameters) []map[string]interface{} {
	config := make(map[string]interface{})

	if redshiftParameters == nil {
		return []map[string]interface{}{config}
	}

	config["database"] = aws.StringValue(redshiftParameters.Database)
	config["db_user"] = aws.StringValue(redshiftParameters.DbUser)
	config["secrets_manager_arn"] = aws.StringValue(redshiftParameters.SecretManagerArn)
	config["sql"] = aws.StringValue(redshiftParameters.Sql)
	config["statement_name"] = aws.StringValue(redshiftParameters.StatementName)
	config["with_event"] = aws.BoolValue(redshiftParameters.WithEvent)

	result := []map[string]interface{}{config}
	return result
}

func flattenAwsCloudWatchEventTargetEcsParametersNetworkConfiguration(nc *events.NetworkConfiguration) []interface{} {
	if nc == nil {
		return nil
	}

	result := make(map[string]interface{})
	result["security_groups"] = flattenStringSet(nc.AwsvpcConfiguration.SecurityGroups)
	result["subnets"] = flattenStringSet(nc.AwsvpcConfiguration.Subnets)

	if nc.AwsvpcConfiguration.AssignPublicIp != nil {
		result["assign_public_ip"] = aws.StringValue(nc.AwsvpcConfiguration.AssignPublicIp) == events.AssignPublicIpEnabled
	}

	return []interface{}{result}
}

func flattenAwsCloudWatchEventTargetBatchParameters(batchParameters *events.BatchParameters) []map[string]interface{} {
	config := make(map[string]interface{})
	config["job_definition"] = aws.StringValue(batchParameters.JobDefinition)
	config["job_name"] = aws.StringValue(batchParameters.JobName)
	if batchParameters.ArrayProperties != nil {
		config["array_size"] = int(aws.Int64Value(batchParameters.ArrayProperties.Size))
	}
	if batchParameters.RetryStrategy != nil {
		config["job_attempts"] = int(aws.Int64Value(batchParameters.RetryStrategy.Attempts))
	}
	result := []map[string]interface{}{config}
	return result
}

func flattenAwsCloudWatchEventTargetKinesisParameters(kinesisParameters *events.KinesisParameters) []map[string]interface{} {
	config := make(map[string]interface{})
	config["partition_key_path"] = aws.StringValue(kinesisParameters.PartitionKeyPath)
	result := []map[string]interface{}{config}
	return result
}

func flattenAwsCloudWatchEventTargetSqsParameters(sqsParameters *events.SqsParameters) []map[string]interface{} {
	config := make(map[string]interface{})
	config["message_group_id"] = aws.StringValue(sqsParameters.MessageGroupId)
	result := []map[string]interface{}{config}
	return result
}

func flattenAwsCloudWatchEventTargetHttpParameters(apiObject *events.HttpParameters) map[string]interface{} {
	if apiObject == nil {
		return nil
	}

	tfMap := map[string]interface{}{}

	if v := apiObject.HeaderParameters; v != nil {
		tfMap["header_parameters"] = aws.StringValueMap(v)
	}

	if v := apiObject.PathParameterValues; v != nil {
		tfMap["path_parameter_values"] = aws.StringValueSlice(v)
	}

	if v := apiObject.QueryStringParameters; v != nil {
		tfMap["query_string_parameters"] = aws.StringValueMap(v)
	}

	return tfMap
}

func flattenAwsCloudWatchInputTransformer(inputTransformer *events.InputTransformer) []map[string]interface{} {
	config := make(map[string]interface{})
	inputPathsMap := make(map[string]string)
	for k, v := range inputTransformer.InputPathsMap {
		inputPathsMap[k] = aws.StringValue(v)
	}
	config["input_template"] = aws.StringValue(inputTransformer.InputTemplate)
	config["input_paths"] = inputPathsMap

	result := []map[string]interface{}{config}
	return result
}

func flatternAwsCloudWatchEventTargetRetryPolicy(rp *events.RetryPolicy) []map[string]interface{} {
	config := make(map[string]interface{})

	config["maximum_event_age_in_seconds"] = aws.Int64Value(rp.MaximumEventAgeInSeconds)
	config["maximum_retry_attempts"] = aws.Int64Value(rp.MaximumRetryAttempts)

	result := []map[string]interface{}{config}
	return result
}

func flatternAwsCloudWatchEventTargetDeadLetterConfig(dlc *events.DeadLetterConfig) []map[string]interface{} {
	config := make(map[string]interface{})

	config["arn"] = aws.StringValue(dlc.Arn)

	result := []map[string]interface{}{config}
	return result
}

func expandAwsCloudWatchEventTargetPlacementConstraints(tfList []interface{}) []*events.PlacementConstraint {
	if len(tfList) == 0 {
		return nil
	}

	var result []*events.PlacementConstraint

	for _, tfMapRaw := range tfList {
		if tfMapRaw == nil {
			continue
		}

		tfMap := tfMapRaw.(map[string]interface{})

		apiObject := &events.PlacementConstraint{}

		if v, ok := tfMap["expression"].(string); ok && v != "" {
			apiObject.Expression = aws.String(v)
		}

		if v, ok := tfMap["type"].(string); ok && v != "" {
			apiObject.Type = aws.String(v)
		}

		result = append(result, apiObject)
	}

	return result
}

func flattenAwsCloudWatchEventTargetPlacementConstraints(pcs []*events.PlacementConstraint) []map[string]interface{} {
	if len(pcs) == 0 {
		return nil
	}
	results := make([]map[string]interface{}, 0)
	for _, pc := range pcs {
		c := make(map[string]interface{})
		c["type"] = aws.StringValue(pc.Type)
		if pc.Expression != nil {
			c["expression"] = aws.StringValue(pc.Expression)
		}

		results = append(results, c)
	}
	return results
}

func resourceAwsCloudWatchEventTargetImport(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
	busName, ruleName, targetID, err := tfevents.TargetParseImportID(d.Id())
	if err != nil {
		return []*schema.ResourceData{}, err
	}

	id := tfevents.TargetCreateID(busName, ruleName, targetID)
	d.SetId(id)
	d.Set("target_id", targetID)
	d.Set("rule", ruleName)
	d.Set("event_bus_name", busName)

	return []*schema.ResourceData{d}, nil
}
