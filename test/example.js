/**
 * User management system in JavaScript
 */

// Constants
const MAX_USERS = 1000;
const DEFAULT_TIMEOUT = 5000;

// Global variables
let userCount = 0;
var isInitialized = false;

/**
 * User class representing a user entity
 */
class User {
    constructor(id, name, email) {
        this.id = id;
        this.name = name;
        this.email = email;
        this.createdAt = new Date();
        this.active = true;
    }

    /**
     * Get user display name
     * @returns {string} Display name
     */
    getDisplayName() {
        return `${this.name} (${this.email})`;
    }

    /**
     * Activate the user
     */
    activate() {
        this.active = true;
    }

    /**
     * Deactivate the user
     */
    deactivate() {
        this.active = false;
    }

    /**
     * Static method to create user from object
     * @param {Object} userData - User data object
     * @returns {User} New user instance
     */
    static fromObject(userData) {
        return new User(userData.id, userData.name, userData.email);
    }

    /**
     * Static method to validate user data
     * @param {Object} userData - User data to validate
     * @returns {boolean} True if valid
     */
    static isValid(userData) {
        return userData && userData.name && userData.email;
    }
}

/**
 * UserService class for managing users
 */
class UserService {
    constructor(repository, logger) {
        this.repository = repository;
        this.logger = logger;
        this.cache = new Map();
    }

    /**
     * Get user by ID
     * @param {number} id - User ID
     * @returns {Promise<User>} User object
     */
    async getUser(id) {
        if (this.cache.has(id)) {
            return this.cache.get(id);
        }

        try {
            const user = await this.repository.findById(id);
            this.cache.set(id, user);
            this.logger.info(`Retrieved user: ${user.name}`);
            return user;
        } catch (error) {
            this.logger.error(`Failed to get user ${id}:`, error);
            throw error;
        }
    }

    /**
     * Create new user
     * @param {Object} userData - User data
     * @returns {Promise<User>} Created user
     */
    async createUser(userData) {
        if (!User.isValid(userData)) {
            throw new Error('Invalid user data');
        }

        const user = User.fromObject(userData);
        const savedUser = await this.repository.save(user);
        userCount++;
        
        this.logger.info(`Created user: ${savedUser.name}`);
        return savedUser;
    }

    /**
     * Update existing user
     * @param {number} id - User ID
     * @param {Object} updateData - Data to update
     * @returns {Promise<User>} Updated user
     */
    async updateUser(id, updateData) {
        const existingUser = await this.getUser(id);
        Object.assign(existingUser, updateData);
        
        const updatedUser = await this.repository.save(existingUser);
        this.cache.set(id, updatedUser);
        
        return updatedUser;
    }

    /**
     * Delete user
     * @param {number} id - User ID
     * @returns {Promise<boolean>} Success status
     */
    async deleteUser(id) {
        await this.repository.delete(id);
        this.cache.delete(id);
        userCount--;
        return true;
    }
}

// Function declarations
function validateEmail(email) {
    const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    return emailRegex.test(email);
}

function generateUserId() {
    return Math.floor(Math.random() * 1000000);
}

/**
 * Initialize the user system
 * @param {Object} config - Configuration object
 */
function initializeUserSystem(config) {
    isInitialized = true;
    userCount = 0;
    console.log('User system initialized with config:', config);
}

// Arrow functions
const formatUser = (user) => ({
    id: user.id,
    name: user.name,
    email: user.email,
    displayName: user.getDisplayName()
});

const createUserValidator = (requiredFields) => (userData) => {
    return requiredFields.every(field => userData[field]);
};

const processUsers = async (users, processor) => {
    const results = [];
    for (const user of users) {
        const result = await processor(user);
        results.push(result);
    }
    return results;
};

// Higher-order function
function createLogger(prefix) {
    return {
        info: (message) => console.log(`[${prefix}] INFO: ${message}`),
        error: (message) => console.error(`[${prefix}] ERROR: ${message}`),
        debug: (message) => console.debug(`[${prefix}] DEBUG: ${message}`)
    };
}

// Factory function
function createUserRepository(type) {
    switch (type) {
        case 'memory':
            return new MemoryUserRepository();
        case 'database':
            return new DatabaseUserRepository();
        default:
            throw new Error(`Unknown repository type: ${type}`);
    }
}

/**
 * Memory-based user repository
 */
class MemoryUserRepository {
    constructor() {
        this.users = new Map();
        this.nextId = 1;
    }

    async findById(id) {
        const user = this.users.get(id);
        if (!user) {
            throw new Error(`User not found: ${id}`);
        }
        return user;
    }

    async save(user) {
        if (!user.id) {
            user.id = this.nextId++;
        }
        this.users.set(user.id, user);
        return user;
    }

    async delete(id) {
        return this.users.delete(id);
    }

    async findAll() {
        return Array.from(this.users.values());
    }
}

// Event handlers
function handleUserClick(event) {
    const userId = event.target.dataset.userId;
    console.log(`User clicked: ${userId}`);
}

function handleFormSubmit(event) {
    event.preventDefault();
    const formData = new FormData(event.target);
    const userData = Object.fromEntries(formData);
    console.log('Form submitted with data:', userData);
}

// Immediately Invoked Function Expression (IIFE)
(function() {
    console.log('User management module loaded');
    
    // Module initialization
    const defaultConfig = {
        maxUsers: MAX_USERS,
        timeout: DEFAULT_TIMEOUT
    };
    
    if (typeof window !== 'undefined') {
        window.UserManagement = {
            User,
            UserService,
            createUserRepository,
            initializeUserSystem
        };
    }
})();

// Export for Node.js
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        User,
        UserService,
        createUserRepository,
        initializeUserSystem,
        validateEmail,
        formatUser
    };
} 