import csv
import json
import os

directory = "/home/pete/Documents/dumps/"
txPortalIncreaseThresh_values = ["224", "112", "56", "28", "14", "7",
                                 "6", "5", "4", "3", "2", "1"]
txPortalStartSize_values = ["16384", "32768", "65536", "131072", "262144",
                            "524288", "1048576", "2097152", "4194304",
                            "8388608", "16777216", "33554432", "67108864"]
tags = [(i, j) for i in txPortalIncreaseThresh_values
        for j in txPortalStartSize_values]
tags_iterator = iter(tags)


def extract_values_from_json(path):
    with open(path, 'r') as json_file:
        data = json.load(json_file)

    host_data = data['regions']['eu-west-2']['hosts']
    extracted_data = {}

    for host_key, host in host_data.items():
        metrics = host['scope']['data'].get('iperf_Flow-Control_metrics')

        if metrics:
            timeslices = metrics['timeslices'][:20]
            bits_per_second_values = [int(slice['bits_per_second'])
                                      for slice in timeslices]
            tag = next(tags_iterator, None)

            new_key = (f"txPortalIncreaseThresh_{tag[0]}-"
                       f"txPortalStartSize_{tag[1]}") if tag else host_key

            extracted_data[new_key] = {
                'bytes': metrics.get('bytes', None),
                'bits_per_second': int(metrics.get('bits_per_second', 0)),
                'timeslice_bits_per_second': bits_per_second_values
            }

    return extracted_data


def process_json_files(d):
    all_data = {}
    json_files = sorted(
        (os.path.join(d, file) for file in os.listdir(d)
         if file.endswith('.json')), key=os.path.getmtime)

    for json_file in json_files:
        all_data.update(extract_values_from_json(json_file))

    return all_data


def count_and_process_json_files(d):
    json_files = [file for file in os.listdir(d) if file.endswith('.json')]
    json_count = len(json_files)

    if json_count % 5 != 0:
        print(f"Warning: Number of JSON files ({json_count}) "
              f"not divisible by 5.")
        exit()
    else:
        print(f"There are {json_count} JSON files in the directory.")
        return process_json_files(d)


data_structures = count_and_process_json_files(directory)

with open('flow_control_data.csv', 'w', newline='') as csv_file:
    writer = csv.writer(csv_file)
    column_names = ['txPortalIncreaseThresh', 'txPortalStartSize',
                    'Total Bytes', 'Total bps'] + [str(i)
                                                   for i in range(1, 21)]
    writer.writerow(column_names)

    for key, values in data_structures.items():
        increase_thresh, start_size = key.split('-')
        row = [increase_thresh.split('_')[-1], start_size.split('_')[-1],
               values.get('bytes'), values.get('bits_per_second')]
        row.extend(values.get('timeslice_bits_per_second', []))
        writer.writerow(row)
        print(' '.join(str(x) if x is not None else '_' for x in row))
