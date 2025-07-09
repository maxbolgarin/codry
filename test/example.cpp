#include <iostream>
#include <string>
#include <vector>
#include <unordered_map>
#include <memory>
#include <optional>
#include <algorithm>
#include <chrono>
#include <stdexcept>

/**
 * User management system in C++
 */

namespace user_management {

// Constants
const int MAX_USERS = 1000;
const std::string DEFAULT_STATUS = "ACTIVE";

// Forward declarations
class User;
class UserRepository;
class UserService;

// Enums
enum class UserStatus {
    ACTIVE,
    INACTIVE,
    SUSPENDED,
    DELETED
};

enum class Permission {
    READ,
    WRITE,
    DELETE,
    ADMIN
};

// Exception classes
class UserNotFoundException : public std::runtime_error {
public:
    explicit UserNotFoundException(int id) 
        : std::runtime_error("User with ID " + std::to_string(id) + " not found") {}
};

class ValidationException : public std::runtime_error {
public:
    explicit ValidationException(const std::string& message) 
        : std::runtime_error("Validation error: " + message) {}
};

class DuplicateEmailException : public std::runtime_error {
public:
    explicit DuplicateEmailException(const std::string& email) 
        : std::runtime_error("User with email " + email + " already exists") {}
};

// Utility functions
std::string statusToString(UserStatus status) {
    switch (status) {
        case UserStatus::ACTIVE: return "ACTIVE";
        case UserStatus::INACTIVE: return "INACTIVE";
        case UserStatus::SUSPENDED: return "SUSPENDED";
        case UserStatus::DELETED: return "DELETED";
        default: return "UNKNOWN";
    }
}

UserStatus stringToStatus(const std::string& status) {
    if (status == "ACTIVE") return UserStatus::ACTIVE;
    if (status == "INACTIVE") return UserStatus::INACTIVE;
    if (status == "SUSPENDED") return UserStatus::SUSPENDED;
    if (status == "DELETED") return UserStatus::DELETED;
    throw std::invalid_argument("Unknown status: " + status);
}

bool isValidEmail(const std::string& email) {
    return email.find('@') != std::string::npos && 
           email.find('.') != std::string::npos &&
           email.length() > 5;
}

// User class
class User {
private:
    int id_;
    std::string name_;
    std::string email_;
    UserStatus status_;
    std::chrono::system_clock::time_point created_at_;
    std::optional<std::chrono::system_clock::time_point> last_login_at_;
    std::vector<Permission> permissions_;
    std::unordered_map<std::string, std::string> metadata_;

public:
    // Constructors
    User() : id_(0), status_(UserStatus::ACTIVE), created_at_(std::chrono::system_clock::now()) {}
    
    User(int id, const std::string& name, const std::string& email)
        : id_(id), name_(name), email_(email), status_(UserStatus::ACTIVE),
          created_at_(std::chrono::system_clock::now()) {}
    
    User(const std::string& name, const std::string& email, 
         const std::vector<Permission>& permissions)
        : id_(0), name_(name), email_(email), status_(UserStatus::ACTIVE),
          created_at_(std::chrono::system_clock::now()), permissions_(permissions) {}

    // Copy constructor
    User(const User& other) = default;
    
    // Move constructor
    User(User&& other) noexcept = default;
    
    // Assignment operators
    User& operator=(const User& other) = default;
    User& operator=(User&& other) noexcept = default;

    // Destructor
    virtual ~User() = default;

    // Getters
    int getId() const { return id_; }
    const std::string& getName() const { return name_; }
    const std::string& getEmail() const { return email_; }
    UserStatus getStatus() const { return status_; }
    const auto& getCreatedAt() const { return created_at_; }
    const auto& getLastLoginAt() const { return last_login_at_; }
    const std::vector<Permission>& getPermissions() const { return permissions_; }
    const std::unordered_map<std::string, std::string>& getMetadata() const { return metadata_; }

    // Setters
    void setId(int id) { id_ = id; }
    void setName(const std::string& name) { name_ = name; }
    void setEmail(const std::string& email) { email_ = email; }
    void setStatus(UserStatus status) { status_ = status; }
    void setPermissions(const std::vector<Permission>& permissions) { permissions_ = permissions; }
    void setMetadata(const std::unordered_map<std::string, std::string>& metadata) { metadata_ = metadata; }

    // Business methods
    void activate() { status_ = UserStatus::ACTIVE; }
    void deactivate() { status_ = UserStatus::INACTIVE; }
    void suspend() { status_ = UserStatus::SUSPENDED; }

    bool hasPermission(Permission permission) const {
        return std::find(permissions_.begin(), permissions_.end(), permission) != permissions_.end() ||
               std::find(permissions_.begin(), permissions_.end(), Permission::ADMIN) != permissions_.end();
    }

    void addPermission(Permission permission) {
        if (!hasPermission(permission)) {
            permissions_.push_back(permission);
        }
    }

    void removePermission(Permission permission) {
        permissions_.erase(
            std::remove(permissions_.begin(), permissions_.end(), permission),
            permissions_.end()
        );
    }

    void updateLastLogin() {
        last_login_at_ = std::chrono::system_clock::now();
    }

    std::string getDisplayName() const {
        return name_ + " (" + email_ + ")";
    }

    bool isActive() const {
        return status_ == UserStatus::ACTIVE;
    }

    // Operators
    bool operator==(const User& other) const {
        return id_ == other.id_;
    }

    bool operator!=(const User& other) const {
        return !(*this == other);
    }

    friend std::ostream& operator<<(std::ostream& os, const User& user) {
        os << "User{id=" << user.id_ 
           << ", name='" << user.name_ 
           << "', email='" << user.email_ 
           << "', status=" << statusToString(user.status_) << "}";
        return os;
    }
};

// Template for repository interface
template<typename T, typename K = int>
class Repository {
public:
    virtual ~Repository() = default;
    virtual std::optional<T> findById(K id) = 0;
    virtual T save(const T& entity) = 0;
    virtual bool deleteById(K id) = 0;
    virtual std::vector<T> findAll() = 0;
};

// User repository interface
class UserRepository : public Repository<User, int> {
public:
    virtual std::optional<User> findByEmail(const std::string& email) = 0;
    virtual std::vector<User> findByStatus(UserStatus status) = 0;
};

// Memory-based repository implementation
class MemoryUserRepository : public UserRepository {
private:
    std::unordered_map<int, User> users_;
    int next_id_;

public:
    MemoryUserRepository() : next_id_(1) {}

    std::optional<User> findById(int id) override {
        auto it = users_.find(id);
        if (it != users_.end()) {
            return it->second;
        }
        return std::nullopt;
    }

    User save(const User& user) override {
        User saved_user = user;
        if (saved_user.getId() == 0) {
            saved_user.setId(next_id_++);
        }
        users_[saved_user.getId()] = saved_user;
        return saved_user;
    }

    bool deleteById(int id) override {
        return users_.erase(id) > 0;
    }

    std::vector<User> findAll() override {
        std::vector<User> result;
        result.reserve(users_.size());
        for (const auto& pair : users_) {
            result.push_back(pair.second);
        }
        return result;
    }

    std::optional<User> findByEmail(const std::string& email) override {
        for (const auto& pair : users_) {
            if (pair.second.getEmail() == email) {
                return pair.second;
            }
        }
        return std::nullopt;
    }

    std::vector<User> findByStatus(UserStatus status) override {
        std::vector<User> result;
        for (const auto& pair : users_) {
            if (pair.second.getStatus() == status) {
                result.push_back(pair.second);
            }
        }
        return result;
    }

    size_t size() const { return users_.size(); }
    void clear() { users_.clear(); next_id_ = 1; }
};

// Create User Request struct
struct CreateUserRequest {
    std::string name;
    std::string email;
    std::vector<Permission> permissions;
    std::unordered_map<std::string, std::string> metadata;

    CreateUserRequest() = default;
    CreateUserRequest(const std::string& n, const std::string& e) 
        : name(n), email(e) {}
};

// Update User Request struct
struct UpdateUserRequest {
    std::optional<std::string> name;
    std::optional<std::string> email;
    std::optional<UserStatus> status;
    std::optional<std::vector<Permission>> permissions;
};

// User service class
class UserService {
private:
    std::unique_ptr<UserRepository> repository_;
    std::unordered_map<int, User> cache_;

    void validateCreateRequest(const CreateUserRequest& request) {
        if (request.name.empty()) {
            throw ValidationException("Name is required");
        }
        if (request.email.empty() || !isValidEmail(request.email)) {
            throw ValidationException("Valid email is required");
        }
    }

public:
    explicit UserService(std::unique_ptr<UserRepository> repository)
        : repository_(std::move(repository)) {}

    User getUser(int id) {
        auto cache_it = cache_.find(id);
        if (cache_it != cache_.end()) {
            return cache_it->second;
        }

        auto user = repository_->findById(id);
        if (!user) {
            throw UserNotFoundException(id);
        }

        cache_[id] = *user;
        return *user;
    }

    User createUser(const CreateUserRequest& request) {
        validateCreateRequest(request);

        auto existing = repository_->findByEmail(request.email);
        if (existing) {
            throw DuplicateEmailException(request.email);
        }

        User user(0, request.name, request.email);
        user.setPermissions(request.permissions);
        user.setMetadata(request.metadata);

        User saved_user = repository_->save(user);
        std::cout << "Created user: " << saved_user.getDisplayName() << std::endl;
        return saved_user;
    }

    User updateUser(int id, const UpdateUserRequest& request) {
        User user = getUser(id);

        if (request.name) user.setName(*request.name);
        if (request.email) {
            if (!isValidEmail(*request.email)) {
                throw ValidationException("Invalid email format");
            }
            user.setEmail(*request.email);
        }
        if (request.status) user.setStatus(*request.status);
        if (request.permissions) user.setPermissions(*request.permissions);

        User updated_user = repository_->save(user);
        cache_[id] = updated_user;
        return updated_user;
    }

    bool deleteUser(int id) {
        User user = getUser(id);
        bool success = repository_->deleteById(id);

        if (success) {
            cache_.erase(id);
            std::cout << "Deleted user: " << user.getDisplayName() << std::endl;
        }

        return success;
    }

    std::vector<User> listUsers() {
        std::vector<User> users = repository_->findAll();
        std::sort(users.begin(), users.end(), 
                  [](const User& a, const User& b) {
                      return a.getCreatedAt() < b.getCreatedAt();
                  });
        return users;
    }

    std::vector<User> searchUsers(const std::string& query) {
        std::vector<User> users = repository_->findAll();
        std::vector<User> result;

        std::string lower_query = query;
        std::transform(lower_query.begin(), lower_query.end(), lower_query.begin(), ::tolower);

        for (const auto& user : users) {
            std::string lower_name = user.getName();
            std::string lower_email = user.getEmail();
            std::transform(lower_name.begin(), lower_name.end(), lower_name.begin(), ::tolower);
            std::transform(lower_email.begin(), lower_email.end(), lower_email.begin(), ::tolower);

            if (lower_name.find(lower_query) != std::string::npos ||
                lower_email.find(lower_query) != std::string::npos) {
                result.push_back(user);
            }
        }

        return result;
    }
};

// User manager class
class UserManager {
private:
    std::unique_ptr<UserService> service_;

public:
    explicit UserManager(std::unique_ptr<UserService> service)
        : service_(std::move(service)) {}

    std::vector<User> bulkCreateUsers(const std::vector<CreateUserRequest>& requests) {
        std::vector<User> created_users;
        created_users.reserve(requests.size());

        for (const auto& request : requests) {
            try {
                User user = service_->createUser(request);
                created_users.push_back(user);
            } catch (const std::exception& e) {
                std::cerr << "Failed to create user " << request.name 
                          << ": " << e.what() << std::endl;
            }
        }

        return created_users;
    }

    std::unordered_map<std::string, int> getUserStats() {
        std::vector<User> users = service_->listUsers();
        std::unordered_map<std::string, int> stats;

        stats["total"] = static_cast<int>(users.size());
        stats["active"] = static_cast<int>(std::count_if(users.begin(), users.end(),
                                                        [](const User& u) { return u.isActive(); }));
        stats["inactive"] = static_cast<int>(std::count_if(users.begin(), users.end(),
                                                          [](const User& u) { return u.getStatus() == UserStatus::INACTIVE; }));
        stats["suspended"] = static_cast<int>(std::count_if(users.begin(), users.end(),
                                                           [](const User& u) { return u.getStatus() == UserStatus::SUSPENDED; }));

        return stats;
    }

    std::string exportUsers() {
        std::vector<User> users = service_->listUsers();
        std::string result = "ID,Name,Email,Status\n";

        for (const auto& user : users) {
            result += std::to_string(user.getId()) + "," +
                     user.getName() + "," +
                     user.getEmail() + "," +
                     statusToString(user.getStatus()) + "\n";
        }

        return result;
    }
};

// Factory functions
std::unique_ptr<UserService> createUserService() {
    auto repository = std::make_unique<MemoryUserRepository>();
    return std::make_unique<UserService>(std::move(repository));
}

// Global variables
static int user_count = 0;
static bool is_initialized = false;

// Initialization function
void initializeSystem() {
    is_initialized = true;
    user_count = 0;
    std::cout << "User management system initialized" << std::endl;
}

} // namespace user_management

// Main function
int main() {
    try {
        using namespace user_management;
        
        initializeSystem();

        auto service = createUserService();
        auto manager = std::make_unique<UserManager>(std::move(service));

        // Create test users
        std::vector<CreateUserRequest> requests = {
            CreateUserRequest("Alice Johnson", "alice@example.com"),
            CreateUserRequest("Bob Smith", "bob@example.com"),
            CreateUserRequest("Charlie Brown", "charlie@example.com")
        };

        auto created_users = manager->bulkCreateUsers(requests);
        std::cout << "Created " << created_users.size() << " users" << std::endl;

        // Get stats
        auto stats = manager->getUserStats();
        std::cout << "User statistics:" << std::endl;
        for (const auto& pair : stats) {
            std::cout << "  " << pair.first << ": " << pair.second << std::endl;
        }

        // Export users
        std::string export_data = manager->exportUsers();
        std::cout << "Exported data:\n" << export_data << std::endl;

    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
} 