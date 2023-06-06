let width = window.innerWidth,
    height = window.innerHeight;

let network = {
    "routers": [
    ],
    "links": [
    ]
};

let svg = d3.select('body').append('svg')
    .attr('width', width)
    .attr('height', height);

let nodes = network.routers,
    links = network.links;

let simulation = d3.forceSimulation(nodes)
    .force('charge', d3.forceManyBody().strength(-2999))
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('link', d3.forceLink(links).distance(() => { return 360; }));

function processMessage(msg) {
    let networkChanged = false;
    let deleteList;
    if(msg.routers != null) {
        console.log("merging routers", msg.routers);
        msg.routers.forEach(id => {
            if(!network.routers.find(o => o.id === id)) {
                network.routers.push({"id": id, "x": 0, "y": 0});
                console.log("added router [" + id + "]");
            }
        });
        deleteList = [];
        network.routers.forEach(r => {
            if(!msg.routers.find(o => o === r.id)) {
                deleteList.push(r.id);
            }
        });
        deleteList.forEach(id => {
            let i = network.routers.findIndex(v => v.id === id);
            if(i > -1) {
                network.routers.splice(i, 1);
                console.log("removed router [" + id + "]");
            }
        });
        console.log("networks.routers", network.routers);
        networkChanged = true;
    }

    if (msg.links != null) {
        console.log("merging links", msg.links);
        msg.links.forEach(l => {
            if (!network.links.find(o => o.id === l.id)) {
                src = network.routers.findIndex(r => r.id === l.src);
                dst = network.routers.findIndex(r => r.id === l.dst);
                if (src > -1 && dst > -1) {
                    network.links.push({"id": l.id, "source": src, "target": dst, "x": 0, "y": 0});
                    console.log("added link [" + l.id + "]");
                }
            }
        });
        deleteList = [];
        network.links.forEach(l => {
            if (!msg.links.find(o => o.id === l.id)) {
                deleteList.push(l.id);
            }
        });
        deleteList.forEach(id => {
            let i = network.links.findIndex(v => v.id === id);
            if (i > -1) {
                network.links.splice(i, 1);
                console.log("removed link [" + id + "]");
            }
        });
        console.log("networks.links", network.links);

        networkChanged = true;
    }

    if (msg.metrics != null) {
        console.log("source", msg.source, "metrics", msg.metrics);
    }

    if(networkChanged) {
        updateSimulation();
    }
}

function updateSimulation() {
    let linkElements = svg.selectAll('line').data(links, link => link.id);
    linkElements.exit().remove();
    let linkEnter = linkElements.enter().append('line').attr('class', 'link');
    linkElements = linkElements.merge(linkEnter);

    let nodeElements = svg.selectAll('circle').data(nodes);
    nodeElements.exit().remove();
    let nodeEnter = nodeElements.enter().append('circle').attr('r', 25).attr('class', 'node');
    nodeElements = nodeElements.merge(nodeEnter);
    nodeElements.raise();

    let textElements = svg.selectAll('text').data(nodes);
    textElements.exit().remove();
    let textEnter = textElements.enter()
        .append('text')
        .text(node => node.id)
        .attr('dx', 0)
        .attr('dy', 0)
        .attr('class', 'node-label');
    textElements = textElements.merge(textEnter);
    textElements.raise();

    simulation.nodes(nodes).on('tick', () => {
        nodeElements.attr('cx', node => node.x).attr('cy', node => node.y);
        textElements.attr('x', node => node.x).attr('y', node => node.y);
        linkElements
            .attr('x1', link => link.source.x)
            .attr('y1', link => link.source.y)
            .attr('x2', link => link.target.x)
            .attr('y2', link => link.target.y);
    });
    simulation.force('link').links(network.links);
    simulation.alpha(1).restart();
}

updateSimulation();