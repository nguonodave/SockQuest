let sock;
let currentUser = "";
let selectedRecipient = null;

const toggleVisibility = (elementId, show) => {
    const el = document.getElementById(elementId);
    el.classList[show ? "remove" : "add"]("hidden");
}

const getInputValue = (id) => document.getElementById(id).value

const clearAndShowChatElements = () => {
    document.getElementById("chatBox").innerHTML = "";
    ["messageInput", "sendButton", "chatBox"].forEach(id => toggleVisibility(id, true));
};

const createMessageElement = (msg) => {
    const p = document.createElement("p");
    p.textContent = msg.from === currentUser ? `You: ${msg.content}` : `${msg.from}: ${msg.content}`;
    p.style.textAlign = msg.from === currentUser ? "right" : "left";
    p.style.color = msg.from === currentUser ? "blue" : "green";
    return p;
};

const showLoginForm = () => {
    toggleVisibility("loginForm", true)
    toggleVisibility("registerForm", false)
};

const showRegisterForm = () => {
    toggleVisibility("registerForm", true)
    toggleVisibility("loginForm", false)
};

document.getElementById("showLogin").onclick = showLoginForm;
document.getElementById("showRegister").onclick = showRegisterForm;

document.getElementById("registerButton").onclick = async () => {
    const username = getInputValue("regUsername")
    const password = getInputValue("regPassword")

    const res = await fetch("/register", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password })
    });

    const data = await res.json();
    alert(data.message);
};

document.getElementById("loginButton").onclick = async () => {
    const username = getInputValue("loginUsername")
    const password = getInputValue("loginPassword")

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
    toggleVisibility("authSection", false)
    toggleVisibility("chatSection", true)
    document.getElementById("currentUser").textContent = currentUser;

    // Initially hide the message input and chat box
    ["messageInput", "sendButton", "chatBox"].forEach(id => toggleVisibility(id, false));

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
            populateUserList(msg.data)
        } else if (msg.to === currentUser && msg.from === selectedRecipient) {
            document.getElementById("chatBox").appendChild(createMessageElement(msg));
        }
    };
}

function populateUserList(users) {
    const userList = document.getElementById("userList");
    userList.innerHTML = "";

    users.forEach(user => {
        if (user.username === currentUser) return;

        const li = document.createElement("li");
        li.textContent = `${user.username} - ${user.status}`;
        li.onclick = () => openConversationWith(user.username);
        userList.appendChild(li);
    });
}

async function openConversationWith(username) {
    selectedRecipient = username;
    clearAndShowChatElements();

    const convRes = await fetch(`/conversation?currentUser=${currentUser}&selectedUser=${selectedRecipient}`);
    const messages = await convRes.json();

    const chatBox = document.getElementById("chatBox");
    messages.forEach(msg => chatBox.appendChild(createMessageElement(msg)));
}

document.getElementById("sendButton").onclick = async () => {
    const content = getInputValue("messageInput")

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

    document.getElementById("chatBox").appendChild(createMessageElement(msg))

    document.getElementById("messageInput").value = "";
};

async function loadUsers() {
    const res = await fetch("/users");
    const users = await res.json();
    populateUserList(users)
}

// session check
window.onload = () => {
    const savedUser = localStorage.getItem("chat_user");
    if (savedUser) {
        currentUser = savedUser;
        startChat();
    }
};