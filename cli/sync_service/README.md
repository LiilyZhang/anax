# Model Management System

## Introduction
The model management system (MMS) is one of the most important parts in Edge Fabric. It is leveraged to ease the burden of AI model management of cognitive services running on an edge node. Edge Fabric provides a set of hzn CLIs to use MMS to manipulate the models object and its metadata.

## MMS CLI
 
### Check MMS status

Before pushing the object, we need to check MMS status using `hzn mms status` command, to make sure MMS is running properly. Check `heathStatus` under `general` and `dbStatus` under `dbHealth`. The values of these two fields should be “green”, which indicate that CSS (cloud sync service part of MMS) and database are both running.

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms status
    {
      "general": {
        "nodeType": "CSS",
        "healthStatus": "green",
        "upTime": 21896
      },
      "dbHealth": {
        "dbStatus": "green",
        "disconnectedFromDB": false,
        "dbReadFailures": 0,
        "dbWriteFailures": 0
      }
    }
    
### Create and publish MMS object

#### Create MMS Object

In MMS, your model file is not published by its own. MMS requires a metadata file along with your model file when publishing and distributing your model file. Metadata file configures a set of attributes of your model. MMS will store, distribute, and retrieve the model objects based on those attributes defined in metadata. 

Metadata file is a json file. You can use `hzn mms object new` to get a template of metadata file. 

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object new
    {
      "objectID": "",            /* Required: A unique identifier of the object. */
      "objectType": "",          /* Required: The type of the object. */
      "destinationOrgID": "$HZN_ORG_ID", /* Required: The organization ID of the object (an object belongs to exactly one organization). */
      "destinationID": "",       /* The node id (without org prefix) where the object should be placed. */
                                 /* If omitted the object is sent to all nodes with the same destinationType. */
                                 /* Delete this field when you are using destinationPolicy. */
      "destinationType": "",     /* The pattern in use by nodes that should receive this object. */
                                 /* If omitted (and if destinationsList is omitted too) the object is broadcast to all known nodes. */
                                 /* Delete this field when you are using policy. */
      "destinationsList": null,  /* The list of destinations as an array of pattern:nodeId pairs that should receive this object. */
                                 /* If provided, destinationType and destinationID must be omitted. */
                                 /* Delete this field when you are using policy. */
      "destinationPolicy": {     /* The policy specification that should be used to distribute this object. */
                                 /* Delete these fields if the target node is using a pattern. */
        "properties": [          /* A list of policy properties that describe the object. */
          {
            "name": "",
            "value": null,
            "type": ""           /* Valid types are string, bool, int, float, list of string (comma separated), version. */
                                 /* Type can be omitted if the type is discernable from the value, e.g. unquoted true is boolean. */
          }
        ],
        "constraints": [         /* A list of constraint expressions of the form <property name> <operator> <property value>, separated by boolean operators AND (&&) or OR (||). */
          ""
        ],
        "services": [            /* The service(s) that will use this object. */
          {
            "orgID": "",         /* The org of the service. */
            "serviceName": "",   /* The name of the service. */
            "arch": "",          /* Set to '*' to indcate services of any hardware architecture. */
            "version": ""        /* A version range. */
          }
        ]
      },
      "expiration": "",          /* A timestamp/date indicating when the object expires (it is automatically deleted). The timestamp should be provided in RFC3339 format.  */
      "version": "",             /* Arbitrary string value. The value is not semantically interpreted. The Model Management System does not keep multiple version of an object. */
      "description": "",         /* An arbitrary description. */
      "activationTime": ""       /* A timestamp/date as to when this object should automatically be activated. The timestamp should be provided in RFC3339 format. */
    }
 
Use `hzn mms object new >> my_metadata.json` to copy the template to a file named `my_metadata.json`.  (Or you can just copy the template from terminal and paste to a file). Then fill out the field in `my_metadata.json`, and save file.

##### Send MMS to node running with policy

My edge node `an12345` is using policy. `usehello2` is one of the services running on it. 
I would like my model to be used by `usehello2` (service script can be found here: https://github.com/open-horizon/anax/blob/0df021091b0348ffe0d491f90dd0386ffe74f54d/test/docker/fs/hzn/service/usehello/start.sh), so I filled out the metadata file like the following:

    {
      "objectID": "lily1",
      "objectType": "model",
      "destinationOrgID": "$HZN_ORG_ID",
      "destinationPolicy": {
        "properties": [
            ],
        "constraints": [],
        "services": [
          {
            "orgID": "e2edev@somecomp.com",
            "arch": "amd64",
            "serviceName": "my.company.com.services.usehello2",
            "version": "1.0.0"
          }
        ]
      },
      "version": "0.0.1",
      "description": "test policy"
    }
    
##### Send MMS to node running with pattern

If you want to send object to the nodes that use patterns, you will need to:

1. specify node pattern name as the `destinationType`. 
1. specify node id as the `destinationID`.  
1. remove destinationPolicy field

Please refer helloMMS as an example (https://github.com/open-horizon/examples/blob/master/edge/services/helloMMS/object.json)

Now, you have both your model file and metadata file ready. Next step is to publish your MMS object those files.

#### Publish MMS object

To publish your model with its metadata, you will use `hzn mms object publish` command

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object publish -m my_metadata.json -f model_file 
    
I check my service log in another terminal using `tail -f /var/log/syslog | grep usehello`. It shows my MMS object is received by the `usehello2` service

    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]: Full poll response: [
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:   {
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "objectID": "lily1",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "objectType": "model",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationOrgID": "userdev",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationID": "an12345",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationType": "",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationsList": [],
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationPolicy": {
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "properties": [],
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "constraints": [],
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "services": [
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:         {
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "orgID": "e2edev@somecomp.com",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "arch": "amd64",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "serviceName": "my.company.com.services.usehello2",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "version": "1.0.0"
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:         }
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       ],
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "timestamp": 1578587102728470113
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     },
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "expiration": "",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "version": "0.0.1",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "description": "test policy",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "link": "",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "inactive": false,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "activationTime": "",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "noData": false,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "metaOnly": false,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationDataUri": "",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "sourceDataUri": "",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "consumers": 1,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "autodelete": false,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "originID": "Cloud",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "originType": "Cloud",
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "deleted": false,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "instanceID": 1578587080905,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "dataID": 1578587080905,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "objectSize": 6,
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "chunkSize": 122880
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:   }
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]: ]
    Jan  9 16:25:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]: Got a new object: lily1

#### List MMS object

hzn mms CLI provides you a command to list your MMS object with given flags. This command will list all objects with its objectID and objectType

    hzn mms object list
    
result of the command:

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object list
    Listing objects in org userdev:
    [
      {
        "objectID": "policy-basicres.tgz",
        "objectType": "model"
      },
      {
        "objectID": "policy-multires.tgz",
        "objectType": "model"
      },
      {
        "objectID": "lily1",
        "objectType": "model"
      }
    ]
    
We can specify objectType and objectID to check the object we just published

    hzn mms object list --objectType=model --objectId=lily1
    
result of the command:

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object list --objectType=model --objectId=lily1
    Listing objects in org userdev:
    [
      {
        "objectID": "lily1",
        "objectType": "model"
      }
    ]
    
To show the full information of MMS object metadata, you can add `-l` to the command

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object list --objectType=model --objectId=lily1 -l
    Listing objects in org userdev:
    [
      {
        "objectID": "lily1",
        "objectType": "model",
        "destinationOrgID": "userdev",
        "destinationID": "",
        "destinationType": "",
        "destinationsList": [],
        "destinationPolicy": {
          "properties": [],
          "constraints": [],
          "services": [
            {
              "orgID": "e2edev@somecomp.com",
              "arch": "amd64",
              "serviceName": "my.company.com.services.usehello2",
              "version": "1.0.0"
            }
          ],
          "timestamp": 1578594025910476311
        },
        "expiration": "",
        "version": "0.0.1",
        "description": "test policy",
        "link": "",
        "inactive": false,
        "activationTime": "",
        "noData": false,
        "metaOnly": false,
        "destinationDataUri": "",
        "sourceDataUri": "",
        "consumers": 1,
        "autodelete": false,
        "originID": "Cloud",
        "originType": "Cloud",
        "deleted": false,
        "instanceID": 1578594025929,
        "dataID": 1578594025929,
        "objectSize": 6,
        "chunkSize": 122880
      }
    ]



If you want to show object status and destinations along with the object, you can add `-d` to the command:

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object list --objectType=model --objectId=lily1 -d
    Listing objects in org userdev:
    [
      {
        "objectID": "lily1",
        "objectType": "model",
        "destinations": [
          {
            "destinationType": "openhorizon.edgenode",
            "destinationID": "an12345",
            "status": "delivered",
            "message": ""
          }
        ],
        "objectStatus": "ready"
      }
    ]
    
Full list of flags can be obtained by `hzn mms object list --help`

    root@lily-test:~/go/src/github.com/open-horizon/anax# hzn mms object list --help
    usage: hzn mms object list [<flags>]

    List objects in the Horizon Model Management Service.

    Flags:
      -h, --help                   Show context-sensitive help (also try --help-long and --help-man).
      -v, --verbose                Verbose output.
          --dry-run                When calling the Horizon or Exchange API, do GETs, but don't do PUTs, POSTs, or DELETEs.
      -o, --org=ORG                The Horizon organization ID. If not specified, HZN_ORG_ID will be used as a default.
      -u, --user-pw=USER:PW        Horizon user credentials to query and create Model Management Service resources. If not specified, HZN_EXCHANGE_USER_AUTH will be used as a default. If you don't prepend it with the user's org, it will
                                   automatically be prepended with the -o value.
      -t, --objectType=OBJECTTYPE  The type of the object to list.
      -i, --objectId=OBJECTID      The id of the object to list. This flag is optional. Omit this flag to list all objects of a given object type.
      -p, --policy=POLICY          Specify true to show only objects using policy. Specify false to show only objects not using policy. If this flag is omitted, both kinds of objects are shown.
      -s, --service=SERVICE        List mms objects using policy that are targetted for the given service. Service specified in the format service-org/service-name.
          --property=PROPERTY      List mms objects using policy that reference the given property name.
          --updateTime=UPDATETIME  List mms objects using policy that has been updated since the given time. The time value is spefified in RFC3339 format: yyyy-MM-ddTHH:mm:ssZ. The time of day may be omitted.
          --destinationType=DESTINATIONTYPE  
                                   List mms objects with given destination type
          --destinationId=DESTINATIONID  
                                   List mms objects with given destination id. Must specify --destinationType to use this flag
          --data=DATA              Specify true to show objects that have data. Specify false to show objects that have no data. If this flag is omitted, both kinds of objects are shown.
      -e, --expirationTime=EXPIRATIONTIME  
                                   List mms objects that expired before the given time. The time value is spefified in RFC3339 format: yyyy-MM-ddTHH:mm:ssZ. Specify now to show objects that are currently expired.
      -l, --long                   Show detailed object metadata information
      -d, --detail                 Provides additional detail about the deployment of the object on edge nodes.



#### Delete MMS object
If you want to delete your MMS object, simply use this hzn command:

    hzn mms object delete --type=TYPE --id=ID

I typed `hzn mms object delete --type=model --id=lily1` in my terminal. Meanwhile, I received message `Acknowledging that Object lily1 is deleted` from service log

    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]: Full poll response: [
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:   {
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "objectID": "lily1",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "objectType": "model",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationOrgID": "userdev",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationID": "an12345",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationType": "",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationsList": [],
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationPolicy": {
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "properties": [],
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "constraints": [],
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "services": [
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:         {
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "orgID": "e2edev@somecomp.com",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "arch": "amd64",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "serviceName": "my.company.com.services.usehello2",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:           "version": "1.0.0"
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:         }
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       ],
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:       "timestamp": 1578587102728470113
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     },
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "expiration": "",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "version": "0.0.1",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "description": "test policy",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "link": "",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "inactive": false,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "activationTime": "",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "noData": false,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "metaOnly": false,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "destinationDataUri": "",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "sourceDataUri": "",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "consumers": 1,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "autodelete": false,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "originID": "Cloud",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "originType": "Cloud",
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "deleted": true,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "instanceID": 1578587080905,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "dataID": 1578587080905,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "objectSize": 6,
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:     "chunkSize": 122880
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]:   }
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]: ]
    Jan  9 16:26:05 lily-test workload-dcaaee7bd128e1820c79835c6e097277d768986cac6762a41ec0861663fa1fb2_amd64_usehello[25429]: Acknowledging that Object lily1 is deleted



