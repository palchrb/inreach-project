// Decode avalanche message
function decodeAvalancheMessage(encodedMessage) {
    if (!encodedMessage || encodedMessage.trim() === "") {
        throw new Error("No message provided.");
    }

    const [codePart, vurderingPart] = encodedMessage.split(";");
    if (!codePart) throw new Error("No encoded message found.");

    const dangerLevels = codePart.slice(0, 3).split("").map(d => parseInt(d, 36));
    const avalancheProblems = [];
    const problemData = codePart.slice(3);

    for (let i = 0; i < problemData.length; i += 9) {
        const segment = problemData.slice(i, i + 9);
        if (segment.length < 9) continue;

        const type = segment[0];
        const cause = segment[1];
        const propagation = segment[2];
        const sensitivity = segment[3];
        const destructiveSize = segment[4];
        const heightCode = segment[5];
        const heightQualifier = segment[6] === "1" ? "Above" : "Up to";
        const directionsCode = segment.slice(7, 9);

        const height = parseInt(heightCode, 36) * 100;
        const directions = decodeDirections(directionsCode);

        avalancheProblems.push({
            type: decodeAvalancheProblemType(type),
            cause: decodeAvalCause(cause),
            propagation: decodeAvalPropagation(propagation),
            sensitivity: decodeAvalTriggerSensitivity(sensitivity),
            destructiveSize: decodeDestructiveSize(destructiveSize),
            height: `${heightQualifier} ${height} masl`,
            directions: directions,
        });
    }

    return { dangerLevels, avalancheProblems, vurdering: vurderingPart || "No assessment available." };
}

function decodeAvalancheProblemType(code) {
    const mapping = {
        "0": "Not given", "1": "New snow (loose)", "2": "Wet snow (loose)",
        "3": "New snow (slab)", "4": "Wind slab", "5": "Persistent weak layer (slab)",
        "6": "Wet snow (slab)", "7": "Glide avalanche"
    };
    return mapping[code] || "Unknown";
}

function decodeAvalCause(code) {
    const mapping = {
        "0": "Not given", "1": "Buried weak layer with new snow",
        "2": "Buried surface hoar", "3": "Buried faceted snow",
        "4": "Poor bonding on smooth crust", "5": "Poor bonding in wind slab",
        "6": "Faceted snow near ground", "7": "Faceted snow above crust",
        "8": "Faceted snow below crust", "9": "Water at ground/melting",
        "a": "Water pooling in snowpack", "b": "Unbonded snow"
    };
    return mapping[code] || "Unknown";
}

function decodeDirections(base36String) {
    if (!/^[0-9a-z]{1,2}$/i.test(base36String)) return "Unknown";
    const binaryString = parseInt(base36String, 36).toString(2).padStart(8, "0");
    const directionsMap = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
    return binaryString.split("").map((bit, index) => (bit === "1" ? directionsMap[index] : null)).filter(Boolean).join(", ");
}

function decodeAvalPropagation(code) {
    const mapping = { "0": "Not given", "1": "Few steep slopes", "2": "Some steep slopes", "3": "Many steep slopes" };
    return mapping[code] || "Unknown";
}

function decodeAvalTriggerSensitivity(code) {
    const mapping = {
        "0": "Not given", "1": "Very hard to trigger", "2": "Hard to trigger",
        "3": "Easy to trigger", "4": "Very easy to trigger", "5": "Natural"
    };
    return mapping[code] || "Unknown";
}

function decodeDestructiveSize(code) {
    const mapping = {
        "0": "Not given", "1": "1 - Small", "2": "2 - Medium", "3": "3 - Large",
        "4": "4 - Very large", "5": "5 - Extreme", "6": "Unknown"
    };
    return mapping[code] || "Unknown";
}

// Direction SVG graphic
function generateDirectionGraphic(directions) {
    const directionsMap = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
    const highlighted = directions.split(", ");
    let svg = `<svg width="80" height="80" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">`;
    svg += `<circle cx="50" cy="50" r="48" stroke="black" fill="white" stroke-width="2"/>`;
    const step = 360 / 8;
    directionsMap.forEach((dir, i) => {
        const s = (step * i - 112.5) * (Math.PI / 180);
        const e = (step * (i + 1) - 112.5) * (Math.PI / 180);
        const x1 = 50 + 48 * Math.cos(s), y1 = 50 + 48 * Math.sin(s);
        const x2 = 50 + 48 * Math.cos(e), y2 = 50 + 48 * Math.sin(e);
        svg += `<path d="M50,50 L${x1},${y1} A48,48 0 0,1 ${x2},${y2} Z" fill="${highlighted.includes(dir) ? "red" : "none"}" stroke="black" stroke-width="1"/>`;
    });
    svg += `</svg>`;
    return svg;
}

// Event listener
document.addEventListener("DOMContentLoaded", () => {
    document.getElementById("decodeAvalancheButton").addEventListener("click", () => {
        const msg1 = document.getElementById("encodedAvalancheMessage1").value.trim();
        const msg2 = document.getElementById("encodedAvalancheMessage2").value.trim();
        const fullMessage = `${msg1}${msg2}`;

        try {
            const decoded = decodeAvalancheMessage(fullMessage);
            document.getElementById("dangerLevels").textContent = `Danger levels: ${decoded.dangerLevels.join(", ")}`;
            document.getElementById("vurdering").textContent = `Assessment: ${decoded.vurdering}`;

            const tableBody = document.getElementById("avalancheTable").querySelector("tbody");
            tableBody.innerHTML = "";
            decoded.avalancheProblems.forEach(problem => {
                const row = document.createElement("tr");
                Object.entries(problem).forEach(([key, value]) => {
                    const cell = document.createElement("td");
                    if (key === "directions") {
                        cell.innerHTML = generateDirectionGraphic(value);
                    } else {
                        cell.textContent = value;
                    }
                    row.appendChild(cell);
                });
                tableBody.appendChild(row);
            });
        } catch (error) {
            alert(error.message);
        }
    });
});
