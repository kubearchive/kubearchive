# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
import urllib3, os, json, random
from datetime import datetime, timedelta
from string import Template
from uuid import uuid4
from pathlib import Path
from locust import HttpUser, task
from cloudevents.http import CloudEvent
from cloudevents.conversion import to_binary

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

token = os.getenv("SA_TOKEN", "")
if token == "":
    raise Exception("SA_TOKEN environment variable is empty")

with open(Path(__file__).parent / "pod.json") as fd:
    template = Template(fd.read())

# Fixed timestamps to ensure consistent parameter values across all executions
FIXED_TIMESTAMP_AFTER = (datetime.now() - timedelta(hours=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
FIXED_TIMESTAMP_BEFORE = datetime.now().strftime("%Y-%m-%dT%H:%M:%SZ")


class GetPods(HttpUser):
    @task
    def get_pods(self):
        # Get pods without filters
        self.client.get(
            "https://localhost:8081/api/v1/pods",
            verify=False,
            headers={"Authorization": f"Bearer {token}"}
        )

    @task
    def get_pods_with_timestamp_after(self):
        # Get pods with fixed creationTimestampAfter filter
        self.client.get(
            f"https://localhost:8081/api/v1/pods?creationTimestampAfter={FIXED_TIMESTAMP_AFTER}",
            verify=False,
            headers={"Authorization": f"Bearer {token}"}
        )

    @task
    def get_pods_with_timestamp_before(self):
        # Get pods with fixed creationTimestampBefore filter
        self.client.get(
            f"https://localhost:8081/api/v1/pods?creationTimestampBefore={FIXED_TIMESTAMP_BEFORE}",
            verify=False,
            headers={"Authorization": f"Bearer {token}"}
        )

    @task
    def get_pods_with_timestamp_range(self):
        # Get pods with both fixed timestamp filters
        self.client.get(
            f"https://localhost:8081/api/v1/pods?creationTimestampAfter={FIXED_TIMESTAMP_AFTER}&creationTimestampBefore={FIXED_TIMESTAMP_BEFORE}",
            verify=False,
            headers={"Authorization": f"Bearer {token}"}
        )

    @task
    def get_pods_wildcard(self):
        """Match pods containing '1' in their name"""
        self.client.get(
            "https://localhost:8081/api/v1/pods?name=*1*",
            verify=False,
            headers={"Authorization": f"Bearer {token}"}
        )


class CreatePods(HttpUser):
    @task
    def create_pod(self):
        now = datetime.now()

        # Generate pod name with numeric suffix for wildcard testing
        numeric_suffix = random.randint(1000000, 9999999)  # 7-digit number
        pod_name = f"pod-{numeric_suffix}"

        data = template.substitute(dict(
            kind="Pod",
            version="v1",
            pod_name=pod_name,
            pod_uuid=uuid4(),
            owner_uuid=uuid4(),
            create_timestamp=now.strftime("%Y-%m-%dT%H:%M:%SZ"),
            update_timestamp=now.strftime("%Y-%m-%dT%H:%M:%SZ"),
            delete_timestamp=now.strftime("%Y-%m-%dT%H:%M:%SZ"),
            resource_version="1",
            namespace="default",
        ))

        attributes = dict(
            source="localhost:443",
            type="org.kubearchive.sinkfilters.resource.archive-when",
            datacontenttype="application/json",
        )
        event = CloudEvent(attributes, json.loads(data))
        headers, body = to_binary(event)
        self.client.post(
            "http://localhost:8082/",
            data=body,
            headers=headers,
        )
