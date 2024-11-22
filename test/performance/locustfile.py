# Copyright KubeArchive Authors
# SPDX-License-Identifier: Apache-2.0
import urllib3, os, json
from datetime import datetime
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


class GetPods(HttpUser):
    @task
    def get_pods(self):
        self.client.get(
            "https://localhost:8081/api/v1/pods",
            verify=False,
            headers={"Authorization": f"Bearer {token}"}
        )


class CreatePods(HttpUser):
    @task
    def create_pod(self):
        now = datetime.now()
        data = template.substitute(dict(
            kind="Pod",
            version="v1",
            pod_name="pod",
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
            type="dev.knative.apiserver.resource.update",
            datacontenttype="application/json",
        )
        event = CloudEvent(attributes, json.loads(data))
        headers, body = to_binary(event)
        self.client.post(
            "http://localhost:8082/",
            data=body,
            headers=headers,
        )
