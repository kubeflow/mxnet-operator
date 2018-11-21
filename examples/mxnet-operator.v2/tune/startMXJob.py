# Befor running this script, make sure tvm is install in your cluster

import os
import time
import json

if __name__ == '__main__':
    mx_config = json.loads(os.environ.get('MX_CONFIG') or '{}')
    cluster_config = mx_config.get('cluster', {})
    labels_config = mx_config.get('labels', {})
    task_config = mx_config.get('task', {})
    task_type = task_config.get('type')
    task_index = task_config.get('index')

    if task_type == "":
        print("No task_type, Error")
    elif task_type == "scheduler" or task_type == "server" or task_type == "worker":
        print("Training task: this python script doesn't matter this procession")
    elif task_type == "tunertracker":
        addr = cluster_config["tunertracker"][0]
        url_port = addr.split(':')
        command = "python3 -m tvm.exec.rpc_tracker --host=0.0.0.0 --port={0}".format(url_port[1])
        print("DO: " + command)
        os.system(command)
    elif task_type == "tunerserver":
        time.sleep(5)
        addr = cluster_config["tunertracker"][0]
        label = labels_config["tunerserver"]
        command = "python3 -m tvm.exec.rpc_server --tracker={0} --key={1}".format(addr, label)
        print("DO: " + command)
        os.system(command)
    elif task_type == "tuner":
        time.sleep(5)
        addr = cluster_config["tunertracker"][0]
        url_port = addr.split(':')
        label = labels_config["tunerserver"]
        command = "python3 /home/test/auto-tuning.py --tracker {0} --tracker_port {1} --server_key {2}".format(url_port[0], url_port[1], label)
        print("DO: " + command)
        os.system(command)
    else:
        print("Unknow task type! Error")