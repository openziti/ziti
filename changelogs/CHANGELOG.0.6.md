# Release 0.6

## Theme

Ziti 0.6.0 moves the back-end persistence model of Ziti Edge and Ziti Fabric into the same
repository based on Bbolt (an in memory data store that is backed by a memory mapped file). The
changes remove the requirement for PostgreSQL.

## UPDB Enrollment JWTs

Enrollments that are for UPDB (username password database) are now consistent with all other
enrollment and use JWTs for processing. Prior to this a naked URL was provided.

### What This Breaks

Any UPDB enrollment processing that relied upon the URL for the enrollment.

Ziti 0.5.x UPDB enrolling entity

```
{
    "meta": {},
    "data": {
        "id": "612843ae-6ac8-48ac-a737-bfc2d28ab9ea",
        "createdAt": "2019-11-21T17:23:00.316631Z",
        "updatedAt": "2019-11-21T17:23:00.316631Z",
        "_links": {
            "self": {
                "href": "./identities/612843ae-6ac8-48ac-a737-bfc2d28ab9ea"
            }
        },
        "tags": {},
        "name": "updb--5badbdc5-e8dd-4877-82df-c06aea7f1197",
        "type": {
            "id": "577104f2-1e3a-4947-a927-7383baefbc9a",
            "name": "User"
        },
        "isDefaultAdmin": false,
        "isAdmin": false,
        "authenticators": {},
        "enrollment": {
            "updb": {
                "username": "asdf",
                "url": "https://demo.ziti.netfoundry.io:1080/enroll?method=updb&token=911e6562-0c83-11ea-a81a-000d3a1b4b17&username=asdf"
            }
        },
        "permissions": []
    }
}
```

Ziti 0.6.x UPDB enrolling entity (note the changes in the enrollment.updb object):

```
{
    "meta": {},
    "data": {
        "id": "39f11c10-0693-41ed-9bec-8011e2721562",
        "createdAt": "2019-11-21T17:28:18.2855234Z",
        "updatedAt": "2019-11-21T17:28:18.2855234Z",
        "_links": {
            "self": {
                "href": "./identities/39f11c10-0693-41ed-9bec-8011e2721562"
            }
        },
        "tags": {},
        "name": "updb--b55f5372-3993-40f5-b534-126e0dd2f1be",
        "type": {
            "entity": "identity-types",
            "id": "577104f2-1e3a-4947-a927-7383baefbc9a",
            "name": "User",
            "_links": {
                "self": {
                    "href": "./identity-types/577104f2-1e3a-4947-a927-7383baefbc9a"
                }
            }
        },
        "isDefaultAdmin": false,
        "isAdmin": false,
        "authenticators": {},
        "enrollment": {
            "updb": {
                "expiresAt": "2019-11-21T17:33:18.2855234Z",
                "issuedAt": "2019-11-21T17:28:18.2855234Z",
                "jwt": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbSI6InVwZGIiLCJleHAiOjE1NzQzNTc1OTgsImlzcyI6Imh0dHBzOi8vbG9jYWxob3N0OjEyODAiLCJqdGkiOiJiYzBlY2NlOC05ZGY0LTQzZDYtYTVhMC0wMjI1MzY2YmM4M2EiLCJzdWIiOiIzOWYxMWMxMC0wNjkzLTQxZWQtOWJlYy04MDExZTI3MjE1NjIifQ.PUcnACCdwqfWRGRzF8lG6xDTgHKAwKV6eTw8tHFuNBXaUNbqExBwUQEW0-cCHsV-nLEyhxyjhXmVCkIDgz-ukKfS0xStiDrJQbiq8m0auodkArmJSsYzElXkKdv37FHu0t-CGoXptdLyuo9eCnzzmci3ev18zMR5HjYMCQEclELV6OEICNr_0EwhAGJa1yX6ODYrLMZ3SdEd6fj-ZGX7j9owTs6iEsqCB_TORfnGGg6lEINE5GlYsyp7JUxolS6H4lPeN5h2mxk2_OkJY8GX3ydv75LsIZ-jjL3xC5XncCESrefgDabib1fudJ4038D0EzqTcOREPAqmjWhnDhTulQ",
                "token": "bc0ecce8-9df4-43d6-a5a0-0225366bc83a"
            }
        },
        "permissions": []
    }
}
```

### What To Do

Use the new JWT format to:

verify the signature of the JWT to match the iss URL's TSL presented certificates construct the
enrollment url from the JWTs properties in the following format:

```
<iss> + "/enroll?token=" + <jti>
```

## Multiple Invalid Value Error Handling

Errors where there is the potential to report about multiple invalid field values for a given field
used to report as a separate error for each value. Now there will be one error, but the values field
will hold the invalid values.

### Old Format

```
{
    "error": {
        "args": {
            "urlVars": {
                "id": "097018b6-108e-42b3-869b-deb9e1814594"
            }
        },
        "cause": {
            "errors": [
                {
                    "message": "entity not found for id [06ecf930-3a9f-4a6c-98b5-8f0be1bde9e2]",
                    "field": "ids[0]",
                    "value": "06ecf930-3a9f-4a6c-98b5-8f0be1bde9e2"
                }
            ]
        },
        "causeMessage": "There were multiple field errors: the value '06ecf930-3a9f-4a6c-9...' for 'ids[0]' is invalid: entity not found for id [06ecf930-3a9f-4a6c-98b5-8f0be1bde9e2]",
        "code": "INVALID_FIELD",
        "message": "The field contains an invalid value",
        "requestId": "48ea4bce-f233-410e-a062-5dbceee20223"
    },
    "meta": {
        "apiEnrolmentVersion": "0.0.1",
        "apiVersion": "0.0.1"
    }
}
```

### New Format

```
{
    "error": {
        "args": {
            "urlVars": {
                "id": "5b15c442-5590-4c58-8bc7-0da788e0cfcf"
            }
        },
        "cause": {
            "message": "clusters(s) not found",
            "field": "clusters",
            "value": [
                "68f8739f-cf52-4d51-9553-dfe7cf9c6a03" 
```
