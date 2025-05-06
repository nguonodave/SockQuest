let sock;
let currentUser = "";
let selectedRecipient = null;

const showLoginForm = () => {
    document.getElementById("loginForm").classList.remove("hidden");
    document.getElementById("registerForm").classList.add("hidden");
};

const showRegisterForm = () => {
    document.getElementById("registerForm").classList.remove("hidden");
    document.getElementById("loginForm").classList.add("hidden");
};

document.getElementById("showLogin").onclick = showLoginForm;
document.getElementById("showRegister").onclick = showRegisterForm;

document.getElementById("registerButton").onclick = async () => {
    const username = document.getElementById("regUsername").value;
    const password = document.getElementById("regPassword").value;

    const res = await fetch("/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password })
    });

    const data = await res.json();
    alert(data.message);
};

document.getElementById("loginButton").onclick = async () => {
    const username = document.getElementById("loginUsername").value;
    const password = document.getElementById("loginPassword").value;

    const res = await fetch("/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password })
    });

    const data = await res.json();

    if (data.success) {
        currentUser = username;
        localStorage.setItem("chat_user", username); // store session
        startChat();
    } else {
        alert(data.message);
    }
};

function startChat() {
    document.getElementById("authSection").classList.add("hidden");
    document.getElementById("chatSection").classList.remove("hidden");
    document.getElementById("currentUser").textContent = currentUser;

    // Initially hide the message input and chat box
    document.getElementById("messageInput").classList.add("hidden");
    document.getElementById("sendButton").classList.add("hidden");
    document.getElementById("chatBox").classList.add("hidden");

    connectWebSocket();
    loadUsers();
}

function connectWebSocket() {
    sock = new WebSocket("ws://localhost:8080/ws");

    sock.onopen = () => {
        sock.send(JSON.stringify({
            from: currentUser,
            to: "",
            content: "",
            timestamp: new Date().toISOString()
        }));
        console.log("WebSocket connected.");
    };

    sock.onmessage = (event) => {
        const msg = JSON.parse(event.data);

        if (msg.type === "userlist") {
            const users = msg.data;
            const userList = document.getElementById("userList");
            userList.innerHTML = "";

            users.forEach(user => {
                if (user.username === currentUser) return;
                const li = document.createElement("li");
                li.textContent = `${user.username} - ${user.status}`;
                li.onclick = async () => {
                    selectedRecipient = user.username;

                    // Clear previous messages
                    document.getElementById("chatBox").innerHTML = "";

                    // Show chat elements
                    document.getElementById("messageInput").classList.remove("hidden");
                    document.getElementById("sendButton").classList.remove("hidden");
                    document.getElementById("chatBox").classList.remove("hidden");

                    // Fetch conversation history
                    const convRes = await fetch(`/conversation?currentUser=${currentUser}&selectedUser=${selectedRecipient}`);
                    const messages = await convRes.json();

                    // Display all messages
                    messages.forEach(msg => {
                        const p = document.createElement("p");
                        if (msg.from === currentUser) {
                            p.textContent = `You: ${msg.content}`;
                            p.style.textAlign = "right";
                            p.style.color = "blue";
                        } else {
                            p.textContent = `${msg.from}: ${msg.content}`;
                            p.style.textAlign = "left";
                            p.style.color = "green";
                        }
                        document.getElementById("chatBox").appendChild(p);
                    });
                };
                userList.appendChild(li);
            });
        } else if (msg.to === currentUser && msg.from === selectedRecipient) {
            // Only show messages from the currently selected recipient
            const p = document.createElement("p");
            p.textContent = `${msg.from}: ${msg.content}`;
            p.style.textAlign = "left";
            p.style.color = "green";
            document.getElementById("chatBox").appendChild(p);
        }
    };
}

document.getElementById("sendButton").onclick = async () => {
    const content = document.getElementById("messageInput").value;

    if (!selectedRecipient || !content) {
        alert("Select a user and type a message.");
        return;
    }

    const msg = {
        from: currentUser,
        to: selectedRecipient,
        content: content,
        timestamp: new Date().toISOString()
    };

    sock.send(JSON.stringify(msg));

    // Immediately show the sent message in the chat
    const p = document.createElement("p");
    p.textContent = `You: ${content}`;
    p.style.textAlign = "right";
    p.style.color = "blue";
    document.getElementById("chatBox").appendChild(p);

    document.getElementById("messageInput").value = "";
};

async function loadUsers() {
    const res = await fetch("/users");
    const users = await res.json();
    const userList = document.getElementById("userList");
    userList.innerHTML = "";

    users.forEach(user => {
        if (user.username === currentUser) return;

        const li = document.createElement("li");
        li.textContent = `${user.username} - ${user.status}`;
        li.onclick = async () => {
            selectedRecipient = user.username;

            // Clear previous messages
            document.getElementById("chatBox").innerHTML = "";

            // Show chat elements
            document.getElementById("messageInput").classList.remove("hidden");
            document.getElementById("sendButton").classList.remove("hidden");
            document.getElementById("chatBox").classList.remove("hidden");

            // Fetch conversation history
            const convRes = await fetch(`/conversation?currentUser=${currentUser}&selectedUser=${selectedRecipient}`);
            const messages = await convRes.json();

            // Display all messages
            messages.forEach(msg => {
                const p = document.createElement("p");
                if (msg.from === currentUser) {
                    p.textContent = `You: ${msg.content}`;
                    p.style.textAlign = "right";
                    p.style.color = "blue";
                } else {
                    p.textContent = `${msg.from}: ${msg.content}`;
                    p.style.textAlign = "left";
                    p.style.color = "green";
                }
                document.getElementById("chatBox").appendChild(p);
            });
        };
        userList.appendChild(li);
    });
}

// session check
window.onload = () => {
    const savedUser = localStorage.getItem("chat_user");
    if (savedUser) {
        currentUser = savedUser;
        startChat();
    }
};