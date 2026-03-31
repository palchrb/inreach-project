// Weather icon mapping: Base85 index -> Yr icon filename
const weatherIconMapping = [
    "01d", "01n", "01m", "02d", "02n", "02m", "03d", "03n", "03m", "04",
    "05d", "05n", "05m", "06d", "06n", "06m", "07d", "07n", "07m", "08d",
    "08n", "08m", "09", "10", "11", "12", "13", "14", "15", "20d", "20n",
    "20m", "21d", "21n", "21m", "22", "23", "24d", "24n", "24m", "25d",
    "25n", "25m", "26d", "26n", "26m", "27d", "27n", "27m", "28d", "28n",
    "28m", "29d", "29n", "29m", "30", "31", "32", "33", "34", "40d", "40n",
    "40m", "41d", "41n", "41m", "42d", "42n", "42m", "43d", "43n", "43m",
    "44d", "44n", "44m", "45d", "45n", "45m", "46", "47", "48", "49", "50"
];

// Register service worker
if ("serviceWorker" in navigator) {
    window.addEventListener("load", () => {
        navigator.serviceWorker.register("service-worker.js")
            .then(() => console.log("Service Worker registered."))
            .catch(err => console.error("SW registration error:", err));
    });
}

const base85Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~";
const base85ToInt = (char) => base85Chars.indexOf(char);

// Decode weather message
function decodeMessage(encodedMessage) {
    if (!encodedMessage || encodedMessage.trim() === "") {
        throw new Error("No message provided");
    }

    const entries = encodedMessage.split(';').filter(Boolean);
    const directions = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"];
    const base36ToInt = (value) => parseInt(value, 36);

    const cityName = decodeURIComponent(entries.shift());
    const dateCode = entries.shift();
    const year = Math.floor(base36ToInt(dateCode) / 10000) + 2000;
    const month = Math.floor((base36ToInt(dateCode) % 10000) / 100);
    const day = base36ToInt(dateCode) % 100;
    const decodedDate = `${day.toString().padStart(2, "0")}.${month.toString().padStart(2, "0")}.${year}`;

    const weatherData = entries.map((entry) => {
        if (entry.length !== 10) {
            throw new Error(`Invalid entry length for: ${entry}`);
        }

        const time = parseInt(entry.slice(0, 1), 36);
        const temp = base36ToInt(entry.slice(1, 3)) - 50;
        const wind = base36ToInt(entry.slice(3, 4));
        const gust = base36ToInt(entry.slice(4, 6));
        const weatherSymbolIndex = base85ToInt(entry.slice(6, 7));

        let weatherIconFile = "01d";
        if (weatherSymbolIndex >= 0 && weatherSymbolIndex < weatherIconMapping.length) {
            weatherIconFile = weatherIconMapping[weatherSymbolIndex];
        }

        const precip = base36ToInt(entry.slice(7, 9)) / 10;
        const direction = directions[base36ToInt(entry.slice(9, 10)) % 8];

        return {
            time: `${time}:00`,
            temp: `${temp}`,
            precip: precip === 0 ? "" : `${precip.toFixed(1)}`,
            wind: `${wind} (${gust})`,
            direction: direction,
            icon: `svg/${weatherIconFile}.svg`
        };
    });

    return { cityName, date: decodedDate, data: weatherData };
}

// localStorage history
const HISTORY_KEY = "weatherDecoderHistory";
const MAX_HISTORY = 10;

function saveToHistory(message) {
    let history = JSON.parse(localStorage.getItem(HISTORY_KEY) || "[]");
    // Remove duplicate if exists
    history = history.filter(h => h.message !== message);
    // Add to front
    const label = message.split(";")[0] + " " + message.split(";")[1];
    history.unshift({ message, label, timestamp: Date.now() });
    // Keep max
    if (history.length > MAX_HISTORY) history = history.slice(0, MAX_HISTORY);
    localStorage.setItem(HISTORY_KEY, JSON.stringify(history));
    renderHistory();
}

function renderHistory() {
    const container = document.getElementById("historyContainer");
    if (!container) return;
    const history = JSON.parse(localStorage.getItem(HISTORY_KEY) || "[]");
    if (history.length === 0) { container.innerHTML = ""; return; }

    container.innerHTML = "<small>Recent:</small> " + history.map(h =>
        `<span class="history-item" data-msg="${encodeURIComponent(h.message)}">${h.label}</span>`
    ).join("");

    container.querySelectorAll(".history-item").forEach(el => {
        el.addEventListener("click", () => {
            const msg = decodeURIComponent(el.dataset.msg);
            document.getElementById("encodedMessage").value = msg;
            document.getElementById("encodedMessagePart2").value = "";
            document.getElementById("decodeButton").click();
        });
    });
}

// Event listener
const decodeButton = document.getElementById("decodeButton");
if (decodeButton) {
    decodeButton.addEventListener("click", () => {
        const part1 = document.getElementById("encodedMessage").value.trim();
        const part2 = document.getElementById("encodedMessagePart2").value.trim();
        const fullMessage = `${part1}${part2}`;

        try {
            const decoded = decodeMessage(fullMessage);
            document.getElementById("weatherDate").textContent = `${decoded.cityName}, ${decoded.date}`;

            const tableBody = document.getElementById("weatherTable").querySelector("tbody");
            tableBody.innerHTML = "";

            decoded.data.forEach((data) => {
                const row = document.createElement("tr");
                [data.time, null, data.temp, data.precip, data.wind, data.direction].forEach((val, i) => {
                    const cell = document.createElement("td");
                    if (i === 1) {
                        const img = document.createElement("img");
                        img.src = data.icon;
                        img.alt = "Weather";
                        img.style.width = "32px";
                        img.style.height = "32px";
                        cell.appendChild(img);
                    } else {
                        cell.textContent = val;
                    }
                    row.appendChild(cell);
                });
                tableBody.appendChild(row);
            });

            saveToHistory(fullMessage);
        } catch (error) {
            alert(error.message);
        }
    });
}

// Render history on load
renderHistory();
