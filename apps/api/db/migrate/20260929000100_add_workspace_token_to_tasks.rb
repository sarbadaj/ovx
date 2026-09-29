class AddWorkspaceTokenToTasks < ActiveRecord::Migration[8.0]
  def change
    add_column :tasks, :workspace_token, :string
    add_index :tasks, :workspace_token, unique: true
  end
end
