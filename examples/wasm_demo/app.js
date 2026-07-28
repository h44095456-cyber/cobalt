document.addEventListener('DOMContentLoaded', () => {
    const codeInput = document.getElementById('code-input');
    const runBtn = document.getElementById('run-btn');
    const sampleSelect = document.getElementById('sample-select');
    const terminalOut = document.getElementById('terminal-out');
    const statusIndicator = document.getElementById('status-indicator');
    const memoryGrid = document.getElementById('memory-grid');

    const presets = {
        math: `fn add(a: int, b: int) -> int:
    return a + b

fn multiply(x: int, y: int) -> int:
    return x * y

fn main():
    let sum = add(40, 2)
    let prod = multiply(6, 7)
    println("Cobalt WebAssembly Target Active!")
    println(sum)
    println(prod)`,
        string: `fn main():
    println("Hello from Cobalt WebAssembly!")
    println("Compiling .cb source directly into WebAssembly WAT/WASM.")
    println(2026)`,
        loop: `fn main():
    var i = 1
    var sum = 0
    while i <= 10:
        sum = sum + i
        i = i + 1
    println("Sum of numbers from 1 to 10:")
    println(sum)`
    };

    sampleSelect.addEventListener('change', (e) => {
        if (presets[e.target.value]) {
            codeInput.value = presets[e.target.value];
        }
    });

    // Populate initial memory inspector cells (64 bytes preview)
    function renderMemoryInspector(buffer) {
        memoryGrid.innerHTML = '';
        const bytes = buffer ? new Uint8Array(buffer, 1024, 64) : new Uint8Array(64);
        bytes.forEach((b, i) => {
            const cell = document.createElement('div');
            cell.className = `mem-cell ${b > 0 ? 'active' : ''}`;
            cell.innerText = b.toString(16).padStart(2, '0').toUpperCase();
            cell.title = `Offset ${1024 + i}: ${b} (${String.fromCharCode(b)})`;
            memoryGrid.appendChild(cell);
        });
    }
    renderMemoryInspector(null);

    runBtn.addEventListener('click', async () => {
        statusIndicator.className = 'status-pill status-running';
        statusIndicator.innerText = 'Compiling...';
        terminalOut.innerHTML = '';
        appendLog('system', '[Compiler] Dispatching Cobalt --target=wasm build pipeline...');

        try {
            // Load pre-compiled app.wasm from current server directory
            const response = await fetch('../../app.wasm');
            if (!response.ok) {
                throw new Error("Could not load app.wasm binary from server.");
            }

            const wasmBytes = await response.arrayBuffer();
            let memory;

            const importObject = {
                env: {
                    println_int: (val) => {
                        appendLog('int', `[WASM Out Int]: ${val.toString()}`);
                    },
                    println_str: (ptr, len) => {
                        const bytes = new Uint8Array(memory.buffer, ptr, len);
                        const str = new TextDecoder().decode(bytes);
                        appendLog('str', `[WASM Out Str]: "${str}"`);
                    },
                    putchar: (ch) => {
                        appendLog('str', String.fromCharCode(ch));
                    }
                }
            };

            const { instance } = await WebAssembly.instantiate(wasmBytes, importObject);
            memory = instance.exports.memory;

            statusIndicator.className = 'status-pill status-ready';
            statusIndicator.innerText = 'Executing';
            appendLog('system', '=== Executing Cobalt WebAssembly main() ===');

            instance.exports.main();

            renderMemoryInspector(memory.buffer);
            appendLog('system', '[System] WebAssembly execution completed cleanly.');
        } catch (err) {
            statusIndicator.className = 'status-pill status-ready';
            statusIndicator.innerText = 'Error';
            appendLog('error', `[Wasm Error]: ${err.message}`);
        }
    });

    function appendLog(type, message) {
        const div = document.createElement('div');
        div.className = `log-line ${type}`;
        div.innerText = message;
        terminalOut.appendChild(div);
        terminalOut.scrollTop = terminalOut.scrollHeight;
    }
});
