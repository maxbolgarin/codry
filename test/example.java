package com.example.user;

import java.util.*;
import java.time.LocalDateTime;
import java.time.format.DateTimeFormatter;
import java.util.concurrent.ConcurrentHashMap;
import java.util.stream.Collectors;

/**
 * User management system in Java
 */
public class UserManagementSystem {
    
    // Constants
    public static final int MAX_USERS = 1000;
    public static final String DEFAULT_STATUS = "ACTIVE";
    private static final DateTimeFormatter DATE_FORMATTER = DateTimeFormatter.ISO_LOCAL_DATE_TIME;
    
    // Static variables
    private static int userCount = 0;
    private static boolean isInitialized = false;
    
    // Enums
    public enum UserStatus {
        ACTIVE("active"),
        INACTIVE("inactive"),
        SUSPENDED("suspended"),
        DELETED("deleted");
        
        private final String value;
        
        UserStatus(String value) {
            this.value = value;
        }
        
        public String getValue() {
            return value;
        }
        
        public static UserStatus fromString(String value) {
            for (UserStatus status : UserStatus.values()) {
                if (status.getValue().equals(value)) {
                    return status;
                }
            }
            throw new IllegalArgumentException("Unknown status: " + value);
        }
    }
    
    public enum Permission {
        READ, WRITE, DELETE, ADMIN
    }
    
    // Exception classes
    public static class UserNotFoundException extends Exception {
        public UserNotFoundException(Long id) {
            super("User with ID " + id + " not found");
        }
    }
    
    public static class ValidationException extends Exception {
        public ValidationException(String message) {
            super("Validation error: " + message);
        }
    }
    
    public static class DuplicateEmailException extends Exception {
        public DuplicateEmailException(String email) {
            super("User with email " + email + " already exists");
        }
    }
    
    // Interfaces
    public interface UserRepository {
        Optional<User> findById(Long id);
        User save(User user);
        boolean deleteById(Long id);
        Optional<User> findByEmail(String email);
        List<User> findAll();
        List<User> findByStatus(UserStatus status);
    }
    
    public interface UserService {
        User getUser(Long id) throws UserNotFoundException;
        User createUser(CreateUserRequest request) throws ValidationException, DuplicateEmailException;
        User updateUser(Long id, UpdateUserRequest request) throws UserNotFoundException, ValidationException;
        boolean deleteUser(Long id) throws UserNotFoundException;
        List<User> listUsers();
        List<User> searchUsers(String query);
    }
    
    // Data Transfer Objects
    public static class CreateUserRequest {
        private String name;
        private String email;
        private Set<Permission> permissions;
        private Map<String, String> metadata;
        
        public CreateUserRequest() {
            this.permissions = new HashSet<>();
            this.metadata = new HashMap<>();
        }
        
        public CreateUserRequest(String name, String email) {
            this();
            this.name = name;
            this.email = email;
        }
        
        // Getters and setters
        public String getName() { return name; }
        public void setName(String name) { this.name = name; }
        
        public String getEmail() { return email; }
        public void setEmail(String email) { this.email = email; }
        
        public Set<Permission> getPermissions() { return permissions; }
        public void setPermissions(Set<Permission> permissions) { this.permissions = permissions; }
        
        public Map<String, String> getMetadata() { return metadata; }
        public void setMetadata(Map<String, String> metadata) { this.metadata = metadata; }
    }
    
    public static class UpdateUserRequest {
        private String name;
        private String email;
        private UserStatus status;
        private Set<Permission> permissions;
        
        // Getters and setters
        public String getName() { return name; }
        public void setName(String name) { this.name = name; }
        
        public String getEmail() { return email; }
        public void setEmail(String email) { this.email = email; }
        
        public UserStatus getStatus() { return status; }
        public void setStatus(UserStatus status) { this.status = status; }
        
        public Set<Permission> getPermissions() { return permissions; }
        public void setPermissions(Set<Permission> permissions) { this.permissions = permissions; }
    }
    
    // User entity class
    public static class User {
        private Long id;
        private String name;
        private String email;
        private UserStatus status;
        private LocalDateTime createdAt;
        private LocalDateTime lastLoginAt;
        private Set<Permission> permissions;
        private Map<String, String> metadata;
        
        // Constructors
        public User() {
            this.status = UserStatus.ACTIVE;
            this.createdAt = LocalDateTime.now();
            this.permissions = new HashSet<>();
            this.metadata = new HashMap<>();
        }
        
        public User(Long id, String name, String email) {
            this();
            this.id = id;
            this.name = name;
            this.email = email;
        }
        
        public User(String name, String email, Set<Permission> permissions) {
            this();
            this.name = name;
            this.email = email;
            this.permissions = new HashSet<>(permissions);
        }
        
        // Business methods
        public void activate() {
            this.status = UserStatus.ACTIVE;
        }
        
        public void deactivate() {
            this.status = UserStatus.INACTIVE;
        }
        
        public void suspend() {
            this.status = UserStatus.SUSPENDED;
        }
        
        public boolean hasPermission(Permission permission) {
            return permissions.contains(permission) || permissions.contains(Permission.ADMIN);
        }
        
        public void addPermission(Permission permission) {
            permissions.add(permission);
        }
        
        public void removePermission(Permission permission) {
            permissions.remove(permission);
        }
        
        public void updateLastLogin() {
            this.lastLoginAt = LocalDateTime.now();
        }
        
        public String getDisplayName() {
            return name + " (" + email + ")";
        }
        
        public boolean isActive() {
            return status == UserStatus.ACTIVE;
        }
        
        // Getters and setters
        public Long getId() { return id; }
        public void setId(Long id) { this.id = id; }
        
        public String getName() { return name; }
        public void setName(String name) { this.name = name; }
        
        public String getEmail() { return email; }
        public void setEmail(String email) { this.email = email; }
        
        public UserStatus getStatus() { return status; }
        public void setStatus(UserStatus status) { this.status = status; }
        
        public LocalDateTime getCreatedAt() { return createdAt; }
        public void setCreatedAt(LocalDateTime createdAt) { this.createdAt = createdAt; }
        
        public LocalDateTime getLastLoginAt() { return lastLoginAt; }
        public void setLastLoginAt(LocalDateTime lastLoginAt) { this.lastLoginAt = lastLoginAt; }
        
        public Set<Permission> getPermissions() { return permissions; }
        public void setPermissions(Set<Permission> permissions) { this.permissions = permissions; }
        
        public Map<String, String> getMetadata() { return metadata; }
        public void setMetadata(Map<String, String> metadata) { this.metadata = metadata; }
        
        @Override
        public boolean equals(Object obj) {
            if (this == obj) return true;
            if (obj == null || getClass() != obj.getClass()) return false;
            User user = (User) obj;
            return Objects.equals(id, user.id);
        }
        
        @Override
        public int hashCode() {
            return Objects.hash(id);
        }
        
        @Override
        public String toString() {
            return "User{" +
                "id=" + id +
                ", name='" + name + '\'' +
                ", email='" + email + '\'' +
                ", status=" + status +
                ", createdAt=" + createdAt +
                '}';
        }
    }
    
    // Repository implementation
    public static class MemoryUserRepository implements UserRepository {
        private final Map<Long, User> users = new ConcurrentHashMap<>();
        private long nextId = 1L;
        
        @Override
        public Optional<User> findById(Long id) {
            return Optional.ofNullable(users.get(id));
        }
        
        @Override
        public User save(User user) {
            if (user.getId() == null) {
                user.setId(nextId++);
            }
            users.put(user.getId(), user);
            return user;
        }
        
        @Override
        public boolean deleteById(Long id) {
            return users.remove(id) != null;
        }
        
        @Override
        public Optional<User> findByEmail(String email) {
            return users.values().stream()
                .filter(user -> email.equals(user.getEmail()))
                .findFirst();
        }
        
        @Override
        public List<User> findAll() {
            return new ArrayList<>(users.values());
        }
        
        @Override
        public List<User> findByStatus(UserStatus status) {
            return users.values().stream()
                .filter(user -> user.getStatus() == status)
                .collect(Collectors.toList());
        }
        
        public int size() {
            return users.size();
        }
        
        public void clear() {
            users.clear();
            nextId = 1L;
        }
    }
    
    // Service implementation
    public static class UserServiceImpl implements UserService {
        private final UserRepository repository;
        private final Map<Long, User> cache = new ConcurrentHashMap<>();
        
        public UserServiceImpl(UserRepository repository) {
            this.repository = repository;
        }
        
        @Override
        public User getUser(Long id) throws UserNotFoundException {
            if (cache.containsKey(id)) {
                return cache.get(id);
            }
            
            User user = repository.findById(id)
                .orElseThrow(() -> new UserNotFoundException(id));
            
            cache.put(id, user);
            return user;
        }
        
        @Override
        public User createUser(CreateUserRequest request) throws ValidationException, DuplicateEmailException {
            validateCreateRequest(request);
            
            if (repository.findByEmail(request.getEmail()).isPresent()) {
                throw new DuplicateEmailException(request.getEmail());
            }
            
            User user = new User(null, request.getName(), request.getEmail());
            user.setPermissions(request.getPermissions());
            user.setMetadata(request.getMetadata());
            
            User savedUser = repository.save(user);
            userCount++;
            
            System.out.println("Created user: " + savedUser.getDisplayName());
            return savedUser;
        }
        
        @Override
        public User updateUser(Long id, UpdateUserRequest request) throws UserNotFoundException, ValidationException {
            User user = getUser(id);
            
            if (request.getName() != null) {
                user.setName(request.getName());
            }
            if (request.getEmail() != null) {
                if (!isValidEmail(request.getEmail())) {
                    throw new ValidationException("Invalid email format");
                }
                user.setEmail(request.getEmail());
            }
            if (request.getStatus() != null) {
                user.setStatus(request.getStatus());
            }
            if (request.getPermissions() != null) {
                user.setPermissions(request.getPermissions());
            }
            
            User updatedUser = repository.save(user);
            cache.put(id, updatedUser);
            
            return updatedUser;
        }
        
        @Override
        public boolean deleteUser(Long id) throws UserNotFoundException {
            User user = getUser(id);
            boolean success = repository.deleteById(id);
            
            if (success) {
                cache.remove(id);
                userCount--;
                System.out.println("Deleted user: " + user.getDisplayName());
            }
            
            return success;
        }
        
        @Override
        public List<User> listUsers() {
            return repository.findAll().stream()
                .sorted(Comparator.comparing(User::getCreatedAt))
                .collect(Collectors.toList());
        }
        
        @Override
        public List<User> searchUsers(String query) {
            String lowerQuery = query.toLowerCase();
            return repository.findAll().stream()
                .filter(user -> 
                    user.getName().toLowerCase().contains(lowerQuery) ||
                    user.getEmail().toLowerCase().contains(lowerQuery))
                .collect(Collectors.toList());
        }
        
        private void validateCreateRequest(CreateUserRequest request) throws ValidationException {
            if (request.getName() == null || request.getName().trim().isEmpty()) {
                throw new ValidationException("Name is required");
            }
            if (request.getEmail() == null || !isValidEmail(request.getEmail())) {
                throw new ValidationException("Valid email is required");
            }
        }
        
        private boolean isValidEmail(String email) {
            return email != null && email.matches("^[A-Za-z0-9+_.-]+@[A-Za-z0-9.-]+\\.[A-Za-z]{2,}$");
        }
    }
    
    // Manager class
    public static class UserManager {
        private final UserService userService;
        
        public UserManager(UserService userService) {
            this.userService = userService;
        }
        
        public List<User> bulkCreateUsers(List<CreateUserRequest> requests) {
            List<User> createdUsers = new ArrayList<>();
            
            for (CreateUserRequest request : requests) {
                try {
                    User user = userService.createUser(request);
                    createdUsers.add(user);
                } catch (Exception e) {
                    System.err.println("Failed to create user " + request.getName() + ": " + e.getMessage());
                }
            }
            
            return createdUsers;
        }
        
        public Map<String, Integer> getUserStats() {
            List<User> users = userService.listUsers();
            Map<String, Integer> stats = new HashMap<>();
            
            stats.put("total", users.size());
            stats.put("active", (int) users.stream().filter(User::isActive).count());
            stats.put("inactive", (int) users.stream().filter(u -> u.getStatus() == UserStatus.INACTIVE).count());
            stats.put("suspended", (int) users.stream().filter(u -> u.getStatus() == UserStatus.SUSPENDED).count());
            
            return stats;
        }
        
        public String exportUsers() {
            List<User> users = userService.listUsers();
            StringBuilder sb = new StringBuilder();
            sb.append("ID,Name,Email,Status,Created At\n");
            
            for (User user : users) {
                sb.append(user.getId()).append(",")
                  .append(user.getName()).append(",")
                  .append(user.getEmail()).append(",")
                  .append(user.getStatus().getValue()).append(",")
                  .append(user.getCreatedAt().format(DATE_FORMATTER))
                  .append("\n");
            }
            
            return sb.toString();
        }
    }
    
    // Utility methods
    public static UserService createUserService() {
        UserRepository repository = new MemoryUserRepository();
        return new UserServiceImpl(repository);
    }
    
    public static void initializeSystem() {
        isInitialized = true;
        userCount = 0;
        System.out.println("User management system initialized");
    }
    
    // Main method for testing
    public static void main(String[] args) {
        try {
            initializeSystem();
            
            UserService service = createUserService();
            UserManager manager = new UserManager(service);
            
            // Create test users
            List<CreateUserRequest> requests = Arrays.asList(
                new CreateUserRequest("Alice Johnson", "alice@example.com"),
                new CreateUserRequest("Bob Smith", "bob@example.com"),
                new CreateUserRequest("Charlie Brown", "charlie@example.com")
            );
            
            List<User> createdUsers = manager.bulkCreateUsers(requests);
            System.out.println("Created " + createdUsers.size() + " users");
            
            // Get stats
            Map<String, Integer> stats = manager.getUserStats();
            System.out.println("User statistics: " + stats);
            
            // Export users
            String exportData = manager.exportUsers();
            System.out.println("Exported data:\n" + exportData);
            
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
} 