
# from botocore.exceptions import ClientError
import json
import logging
import os
import sys

import jinja2
import requests
import yaml

logger = logging.getLogger()
logger.addHandler(logging.StreamHandler())  # stdout
logger.setLevel(logging.DEBUG)
logging.getLogger('boto3').setLevel(logging.CRITICAL)
logging.getLogger('botocore').setLevel(logging.CRITICAL)
logging.getLogger('urllib3').setLevel(logging.CRITICAL)

import boto3

client = boto3.client('cloudfront')

CF_HOST = 'get.openziti.io'
CF_FUNCTION_NAME = 'github-raw-viewer-request-router'
CF_FUNCTION_TEMPLATE = 'dist/cloudfront/get.openziti.io/cloudfront-function-get-openziti-io.js.j2'
GITHUB_SHA = os.environ['GITHUB_SHA']
CF_ROUTES_FILE = 'dist/cloudfront/get.openziti.io/routes.yml'
routes = yaml.safe_load(open(CF_ROUTES_FILE, 'r').read())
jinja2_env = jinja2.Environment()

unique_routes = dict()
# validate the shape of routes and compute the regex and backreference for each
for route in routes:
    if bool(unique_routes.get(route['get'])):
        raise ValueError(f"route 'get' path '{route['get']}' is not unique")
    else:
        unique_routes[route['get']] = True
    # ensure the generated regex is valid for matching HTTP request paths from the viewer
    if not route['get'].startswith('/'):
        raise ValueError(f"route 'get' path '{route['get']}' must start with '/'")
    # ensure the destination uri so we can append the backreference to compose a valid HTTP request path to the origin
    elif not route['raw'].endswith('/'):
        raise ValueError(f"GitHub raw path '{route['raw']}' must end with '/'")
    # is a directory shortcut, so it must have a file to test the route
    elif route['get'].endswith('/'):
        if not bool(route.get('file')):
            raise ValueError(f"route 'get' path '{route['get']}' ends with '/', but no test file is specified")
        route['re'] = f"^\\{route['get'][0:-1]}\\/(.*)"
    # is a file shortcut, so the file is the route
    else:
        if bool(route.get('file')):
            raise ValueError(f"route 'get' '{route['get']}' does not end with '/', so no file may be specified because the 'get' path is the test file")
        route['re'] = f"^\\/({route['get'][1:]})(\\?.*)?$"
        route['file'] = route['get'][1:]
    # always append backreference to the destination uri to compose a valid HTTP request path to the origin ending with
    # the matching part of the request regex
    route['raw'] = f"{route['raw']}$1"
    # render the raw path as a jinja2 template to allow for dynamic values
    route['raw'] = jinja2_env.from_string(route['raw']).render(GITHUB_SHA=GITHUB_SHA)

cf_function_template_bytes = open(CF_FUNCTION_TEMPLATE, 'rb').read()
cf_function_template = jinja2_env.from_string(cf_function_template_bytes.decode('utf-8'))
cf_function_rendered = cf_function_template.render(routes=routes)

# TODO: revert or comment after local testing
# tmp = open('/tmp/rendered.js', 'w')
# tmp.write(cf_function_rendered)
# tmp.close()

# find the dev function etag to update
describe_dev_function = client.describe_function(
    Name=CF_FUNCTION_NAME,
    Stage='DEVELOPMENT'
)
logger.debug(f"got dev function etag: {describe_dev_function['ETag']}")

# update the dev function and capture the new etag as a candidate for promotion
update_response = client.update_function(
    Name=CF_FUNCTION_NAME,
    IfMatch=describe_dev_function['ETag'],
    FunctionConfig={
        'Comment': f"Repo 'openziti/zti' GitHub SHA: {GITHUB_SHA}",
        'Runtime': 'cloudfront-js-1.0',
    },
    FunctionCode=cf_function_rendered,
)
candidate_function_etag = update_response['ETag']
logger.debug(f"got candidate etag: {candidate_function_etag}")


# verify a random /path is handled correctly for each route by the candidate function
def test_route(client: object, etag: str, route: dict):
    if route['get'].endswith('/'):
        # the shortcut is a directory, so append the test file to the path
        request_uri = f"{route['get']}{route['file']}"
    else:
        # the get shortcut is a file
        request_uri = route['get']

    test_obj = {
        "version": "1.0",
        "context": {
            "eventType": "viewer-request"
        },
        "viewer": {
            "ip": "1.2.3.4"
        },
        "request": {
            "method": "GET",
            "uri": request_uri,
            "headers": {
                "host": {"value": CF_HOST}
            },
            "cookies": {},
            "querystring": {}
        }
    }

    test_response = client.test_function(
        Name=CF_FUNCTION_NAME,
        IfMatch=etag,
        Stage='DEVELOPMENT',
        EventObject=bytes(json.dumps(test_obj, default=str), 'utf-8')
    )
    test_result = json.loads(test_response['TestResult']['FunctionOutput'])
    test_result_uri = test_result['request']['uri']
    logger.debug(f"got test result uri: {test_result_uri}")
    full_expected_path = f"{os.path.dirname(route['raw'])}/{route['file']}"
    if test_result_uri == full_expected_path:
        logger.debug(f"Test path '{request_uri}' passed, got expected uri {full_expected_path}")
    else:
        raise ValueError(f"Test path '{request_uri}' failed, got unexpected uri {test_result_uri}, expected {full_expected_path}")

    # if a file is expected then independently verify it exists in Github
    file_result = requests.get(f'https://raw.githubusercontent.com{full_expected_path}')
    file_result.raise_for_status()


for route in routes:
    test_route(client=client, etag=candidate_function_etag, route=route)

if len(sys.argv) > 1 and sys.argv[1] == '--no-publish':
    logger.info(f"not publishing function: {CF_FUNCTION_NAME}")
else:
    # promote candidate from DEVELOPMENT to LIVE
    publish_response = client.publish_function(
        Name=CF_FUNCTION_NAME,
        IfMatch=candidate_function_etag
    )
    logger.info(f"published function: {publish_response['FunctionSummary']['Name']}")

#
# scratch
#

# distributions = client.list_distributions()
# cf_distribution_id = None
# for distribution in distributions['DistributionList']['Items']:
#     if distribution['Aliases']['Items'][0] == 'docs.openziti.io':
#         cf_distribution_id = distribution['Id']
#         print(f"Found docs distribution id: {cf_distribution_id}")
