
# from botocore.exceptions import ClientError
import json
import logging
import os
import random
import string
import requests

import jinja2

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
CF_ROUTES = {
    'qs': {
        're': r'(^\/qs\/)(.*)',
        'uri': f'/openziti/ziti/{GITHUB_SHA}/quickstart/$2',
        'file': 'kubernetes/miniziti.bash',
    },
    'quick': {
        're': r'(^\/quick\/)(.*)',
        'uri': f'/openziti/ziti/{GITHUB_SHA}/quickstart/docker/image/$2',
        'file': 'ziti-cli-functions.sh',
    },
    'dock': {
        're': r'(^\/dock\/)(.*)',
        'uri': f'/openziti/ziti/{GITHUB_SHA}/quickstart/docker/$2',
        'file': 'docker-compose.yml',
    },
    'spec': {
        're': r'(^\/spec\/)(.*)',
        'uri': '/openziti/edge-api/main/$2',
        'file': 'management.yml',
    },
    'pack': {
        're': r'(^\/(tun|pack)\/)(.*)',
        'uri': '/openziti/ziti-tunnel-sdk-c/main/$3',
        'file': 'package-repos.gpg',
    },
}
cf_function_template_bytes = open(CF_FUNCTION_TEMPLATE, 'rb').read()
jinja2_env = jinja2.Environment()
cf_function_template = jinja2_env.from_string(cf_function_template_bytes.decode('utf-8'))
cf_function_rendered = cf_function_template.render(routes=CF_ROUTES)
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
def test_route(client: object, etag: str, requested: str, expected_path_prefix: str, expected_file: str = None):
    if expected_file:
        path_file = expected_file
    else:
        path_file = f"{''.join(random.choices(string.ascii_uppercase+string.digits, k=4))}.html"

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
            "uri": f"/{requested}/{path_file}",
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
    full_expected_path = f'{os.path.dirname(expected_path_prefix)}/{path_file}'
    if test_result_uri == full_expected_path:
        logger.debug(f"Test path '/{requested}/{path_file}' passed, got expected uri {full_expected_path}")
    else:
        logger.error(f"Test path '/{requested}/{path_file}' failed, got unexpected uri {test_result_uri}, expected {full_expected_path}")
        exit(1)

    # if a file is expected then independently verify it exists in Github
    if expected_file:
        file_result = requests.get(f'https://raw.githubusercontent.com{full_expected_path}')
        file_result.raise_for_status()


for name, data in CF_ROUTES.items():
    test_route(client=client, etag=candidate_function_etag, requested=name, expected_path_prefix=data['uri'], expected_file=data['file'])

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
